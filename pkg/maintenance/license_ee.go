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

func NewCheckLicenseJob(store db.Implementor, config common.ConfigStore) common.PeriodicJob {
	keys, _ := license.ActivationKeys()

	return &checkLicenseJob{
		Store:      store,
		Keys:       keys,
		URL:        config.Get(common.EnterpriseUrlKey),
		LicenseKey: config.Get(common.EnterpriseLicenseKeyKey),
		Email:      config.Get(common.EnterpriseEmailKey),
		AdminEmail: config.Get(common.AdminEmailKey),
	}
}

type checkLicenseJob struct {
	Store      db.Implementor
	Keys       []*license.ActivationKey
	URL        common.ConfigItem
	LicenseKey common.ConfigItem
	Email      common.ConfigItem
	AdminEmail common.ConfigItem
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
	url := j.URL.Value()
	if len(url) == 0 {
		return nil, errEnterpriseConfigError
	}

	licenseKey := j.LicenseKey.Value()
	if len(licenseKey) == 0 {
		return nil, errEnterpriseConfigError
	}

	email := j.Email.Value()
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

func (j *checkLicenseJob) RunOnce(ctx context.Context) error {
	if len(j.Keys) == 0 {
		slog.ErrorContext(ctx, "No license keys available")
		return errEnterpriseConfigError
	}

	tnow := time.Now().UTC()
	hadCached := false

	if data, err := j.Store.Impl().RetrieveFromCache(ctx, activationCacheKey); err == nil {
		hadCached = true

		if msg, err := license.VerifyActivation(ctx, data, j.Keys, tnow); err == nil {
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
		slog.WarnContext(ctx, "License is not cached", common.ErrAttr(err))
	}

	data, err := j.fetchActivation(ctx)
	if err != nil {
		if hadCached {
			adminEmail := j.AdminEmail.Value()
			if admin, aerr := j.Store.Impl().FindUserByEmail(ctx, adminEmail); aerr == nil {
				duration := 7 * 24 * time.Hour
				text := fmt.Sprintf("Failed to renew EE license (%s): <i>%s</i>", tnow.Format(time.DateOnly), err.Error())
				_, _ = j.Store.Impl().CreateNotification(ctx, text, tnow, &duration, &admin.ID)
			} else {
				slog.ErrorContext(ctx, "Failed to find admin user by email", "email", adminEmail, common.ErrAttr(aerr))
			}
		}
		return err
	}

	msg, err := license.VerifyActivation(ctx, data, j.Keys, tnow)
	if err == nil {
		slog.InfoContext(ctx, "Received activation is valid")
		_ = j.Store.Impl().StoreInCache(ctx, activationCacheKey, data, msg.Expiration.Sub(tnow))
	} else {
		slog.ErrorContext(ctx, "Failed to verify activation", common.ErrAttr(err))
	}

	return err
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
