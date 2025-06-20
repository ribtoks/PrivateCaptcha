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

func (j *WarmupAPICacheJob) RunOnce(ctx context.Context) error {
	// TODO: Switch to a percentile in future
	users, properties, err := j.TimeSeries.RetrieveRecentTopUsers(ctx, j.Limit)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve top users", common.ErrAttr(err))
		return err
	}

	for userID := range users {
		if _, err := j.Store.Impl().RetrieveUserAPIKeys(ctx, userID); err != nil {
			slog.ErrorContext(ctx, "Failed to retrieve user API keys", "userID", userID, common.ErrAttr(err))
		} else {
			time.Sleep(j.Backoff)
		}
	}

	if _, err := j.Store.Impl().RetrievePropertiesByID(ctx, properties); err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve properties", common.ErrAttr(err))
	}

	slog.InfoContext(ctx, "Warmed up API cache", "users", len(users), "properties", len(properties))

	return nil
}
