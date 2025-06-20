package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/session"
	"github.com/jackc/pgx/v5/pgtype"
)

type SessionsCleanupJob struct {
	Session *session.Manager
}

var _ common.PeriodicJob = (*SessionsCleanupJob)(nil)

func (j *SessionsCleanupJob) Interval() time.Duration {
	return j.Session.MaxLifetime
}

func (j *SessionsCleanupJob) Jitter() time.Duration {
	return 1
}

func (j *SessionsCleanupJob) Name() string {
	return "sessions_cleanup_job"
}

func (j *SessionsCleanupJob) RunOnce(ctx context.Context) error {
	j.Session.GC(ctx)

	return nil
}

type WarmupPortalAuthJob struct {
	Store               db.Implementor
	RegistrationAllowed bool
}

var _ common.OneOffJob = (*WarmupPortalAuthJob)(nil)

func (j *WarmupPortalAuthJob) Name() string {
	return "warmup_portal_auth_job"
}

func (j *WarmupPortalAuthJob) InitialPause() time.Duration {
	return 5 * time.Second
}

func (j *WarmupPortalAuthJob) RunOnce(ctx context.Context) error {
	loginUUID := pgtype.UUID{}
	var err error
	if err = loginUUID.Scan(db.PortalLoginPropertyID); err == nil {
		loginSitekey := db.UUIDToSiteKey(loginUUID)
		if _, err = j.Store.Impl().RetrievePropertyBySitekey(ctx, loginSitekey); err != nil {
			slog.ErrorContext(ctx, "Failed to retrieve login property by sitekey", common.ErrAttr(err))
		}
	}

	if err != nil {
		return err
	}

	if j.RegistrationAllowed {
		registerUUID := pgtype.UUID{}
		if err = registerUUID.Scan(db.PortalRegisterPropertyID); err == nil {
			registerSitekey := db.UUIDToSiteKey(registerUUID)
			if _, err = j.Store.Impl().RetrievePropertyBySitekey(ctx, registerSitekey); err != nil {
				slog.ErrorContext(ctx, "Failed to retrieve register property by sitekey", common.ErrAttr(err))
			}
		}
	}

	return err
}
