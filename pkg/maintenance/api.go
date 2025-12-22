package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
)

type WarmupAPICacheJob struct {
	Store      db.Implementor
	TimeSeries common.TimeSeriesStore
	Backoff    time.Duration
	Limit      int
}

func (j *WarmupAPICacheJob) Name() string {
	return "warmup_api_cache_job"
}

func (j *WarmupAPICacheJob) InitialPause() time.Duration {
	return 5 * time.Second
}

func (j *WarmupAPICacheJob) NewParams() any {
	return struct{}{}
}

func (j *WarmupAPICacheJob) RunOnce(ctx context.Context, params any) error {
	// TODO: Switch to a percentile in future
	propertiesMap, err := j.TimeSeries.RetrieveRecentTopProperties(ctx, j.Limit)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve top users", common.ErrAttr(err))
		return err
	}

	properties, err := j.Store.Impl().RetrievePropertiesByID(ctx, propertiesMap)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve properties", common.ErrAttr(err))
		return err
	}

	t := struct{}{}
	users := make(map[int32]struct{}, len(properties))
	for _, p := range properties {
		if p.OrgOwnerID.Valid {
			users[p.OrgOwnerID.Int32] = t
		}
		if p.CreatorID.Valid && (!p.OrgOwnerID.Valid || (p.CreatorID.Int32 != p.OrgOwnerID.Int32)) {
			users[p.CreatorID.Int32] = t
		}
	}

	for userID := range users {
		if _, err := j.Store.Impl().RetrieveUserAPIKeys(ctx, userID); err != nil {
			slog.ErrorContext(ctx, "Failed to retrieve user API keys", "userID", userID, common.ErrAttr(err))
		} else {
			time.Sleep(j.Backoff)
		}
	}

	slog.InfoContext(ctx, "Warmed up API cache", "users", len(users), "properties", len(properties))

	return nil
}
