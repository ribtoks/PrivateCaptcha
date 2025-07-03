//go:build enterprise

package maintenance

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/license"
	"github.com/jpillora/backoff"
	"github.com/rs/xid"
)

const (
	activationCacheKey    = "license_activation"
	activationAPIAttempts = 5
)

var (
	errLicenseRequest        = errors.New("license request error")
	errLicenseServer         = errors.New("license server error")
	errEnterpriseConfigError = errors.New("enterprise config error")
)

func NewCheckLicenseJob(store db.Implementor, config common.ConfigStore, quitFunc func(ctx context.Context)) (common.PeriodicJob, error) {
	keys, err := license.ActivationKeys()
	if err != nil {
		return nil, err
	}

	return &checkLicenseJob{
		store:      store,
		keys:       keys,
		url:        config.Get(common.EnterpriseUrlKey),
		licenseKey: config.Get(common.EnterpriseLicenseKeyKey),
		email:      config.Get(common.EnterpriseEmailKey),
		adminEmail: config.Get(common.AdminEmailKey),
		quitFunc:   quitFunc,
	}, nil
}

type checkLicenseJob struct {
	store      db.Implementor
	keys       []*license.ActivationKey
	url        common.ConfigItem
	licenseKey common.ConfigItem
	email      common.ConfigItem
	adminEmail common.ConfigItem
	quitFunc   func(ctx context.Context)
}

var _ common.PeriodicJob = (*checkLicenseJob)(nil)

func (j *checkLicenseJob) doFetchActivation(ctx context.Context, licenseURL, licenseKey, email string) ([]byte, error) {
	form := url.Values{}
	form.Set("lid", licenseKey)
	form.Set("email", email)

	req, err := http.NewRequest(http.MethodPost, licenseURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}

	rid := xid.New().String()
	req.Header.Set(common.HeaderContentType, common.ContentTypeURLEncoded)
	req.Header.Set(common.HeaderRequestID, rid)

	rlog := slog.With("requestID", rid)
	rlog.DebugContext(ctx, "Sending license request", "URL", licenseURL)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	const maxBytes = 256 * 1024
	limitedReader := io.LimitReader(resp.Body, maxBytes)
	responseData, responseErr := io.ReadAll(limitedReader)

	// we _can_ retry on server errors and rate limiting
	if (resp.StatusCode >= 500) || (resp.StatusCode == http.StatusTooManyRequests) {
		rlog.WarnContext(ctx, "Failed to fetch activation", "code", resp.StatusCode, "response", string(responseData))
		return nil, errLicenseServer
	}

	// the difference is that we don't retry on most 4xx (client) errors (e.g. BadRequest)
	if resp.StatusCode >= 400 {
		rlog.WarnContext(ctx, "Failed to fetch activation", "code", resp.StatusCode, "response", string(responseData))

		return nil, errLicenseRequest
	}

	rlog.DebugContext(ctx, "Received license response", "code", resp.StatusCode, "response", len(responseData))

	return responseData, responseErr
}

func (j *checkLicenseJob) fetchActivation(ctx context.Context) ([]byte, error) {
	url := j.url.Value()
	if len(url) == 0 {
		return nil, errEnterpriseConfigError
	}

	licenseKey := j.licenseKey.Value()
	if len(licenseKey) == 0 {
		return nil, errEnterpriseConfigError
	}

	email := j.email.Value()
	if len(email) == 0 {
		return nil, errEnterpriseConfigError
	}

	b := &backoff.Backoff{
		Min:    1 * time.Second,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	var data []byte
	var err error

	for i := 0; i < activationAPIAttempts; i++ {
		data, err = j.doFetchActivation(ctx, url, licenseKey, email)
		if (err == nil) || (err == errLicenseRequest) {
			break
		} else {
			slog.WarnContext(ctx, "Failed to fetch activation", "attempt", i, common.ErrAttr(err))
			time.Sleep(b.Duration())
		}
	}

	return data, err
}

func (j *checkLicenseJob) activateLicense(ctx context.Context, tnow time.Time) error {
	data, err := j.fetchActivation(ctx)
	if err != nil {
		return err
	}

	msg, err := license.VerifyActivation(ctx, data, j.keys, tnow)
	if err == nil {
		slog.InfoContext(ctx, "Server activation is valid")
		_ = j.store.Impl().StoreInCache(ctx, activationCacheKey, data, msg.Expiration.Sub(tnow))
	} else {
		slog.ErrorContext(ctx, "Failed to verify server activation", common.ErrAttr(err))
	}

	return err
}

func (j *checkLicenseJob) checkLicense(ctx context.Context) error {
	if len(j.keys) == 0 {
		slog.ErrorContext(ctx, "No license keys available")
		return errEnterpriseConfigError
	}

	tnow := time.Now().UTC()
	cachedIsValid := false

	if data, err := j.store.Impl().RetrieveFromCache(ctx, activationCacheKey); err == nil {
		if msg, err := license.VerifyActivation(ctx, data, j.keys, tnow); err == nil {
			cachedIsValid = true
			expiration := msg.Expiration.Sub(tnow)
			slog.InfoContext(ctx, "Cached activation is valid", "expiration", expiration.String())
			if expiration.Hours() > 24*7 {
				return nil
			}
			// else we will proceed below to actually fetch it again
		} else {
			slog.WarnContext(ctx, "Failed to verify cached activation", common.ErrAttr(err))
		}
	} else {
		slog.WarnContext(ctx, "Activation is not cached", common.ErrAttr(err))
	}

	if err := j.activateLicense(ctx, tnow); err != nil {
		if cachedIsValid {
			// create warning, but swallow the error
			adminEmail := j.adminEmail.Value()
			if admin, aerr := j.store.Impl().FindUserByEmail(ctx, adminEmail); aerr == nil {
				// truncating time will cause duplicate notification being rejected based on SQL constraint
				notifTime := tnow.Truncate(24 * time.Hour)
				notifDuration := 7 * 24 * time.Hour
				text := fmt.Sprintf("Failed to renew EE license (%s): <i>%s</i>", tnow.Format(time.DateOnly), err.Error())
				_, _ = j.store.Impl().CreateNotification(ctx, text, notifTime, &notifDuration, &admin.ID)
			} else {
				slog.ErrorContext(ctx, "Failed to find admin user by email", "email", adminEmail, common.ErrAttr(aerr))
			}

			return nil
		}

		return err
	}

	return nil
}

func (j *checkLicenseJob) RunOnce(ctx context.Context) error {
	if err := j.checkLicense(ctx); err != nil {
		go j.quitFunc(ctx)
		return err
	}

	return nil
}

func (j *checkLicenseJob) Interval() time.Duration {
	return 24 * time.Hour
}

func (j *checkLicenseJob) Jitter() time.Duration {
	return 1
}

func (j *checkLicenseJob) Name() string {
	return "CheckLicenseJob"
}
