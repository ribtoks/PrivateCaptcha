package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	"github.com/jackc/pgx/v5/pgtype"
)

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

func (j *WarmupPortalAuthJob) NewParams() any {
	return struct{}{}
}

func (j *WarmupPortalAuthJob) RunOnce(ctx context.Context, params any) error {
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
