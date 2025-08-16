package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/billing"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	"github.com/jpillora/backoff"
)

const (
	maxSoftDeletedProperties    = 30
	maxSoftDeletedOrganizations = 30
	maxSoftDeletedUsers         = 30
)

type GarbageCollectDataJob struct {
	Age        time.Duration
	BusinessDB db.Implementor
	TimeSeries common.TimeSeriesStore
}

var _ common.PeriodicJob = (*GarbageCollectDataJob)(nil)

func (j *GarbageCollectDataJob) Interval() time.Duration {
	return 1 * time.Hour
}

func (j *GarbageCollectDataJob) Jitter() time.Duration {
	return 1 * time.Hour
}

func (j *GarbageCollectDataJob) Name() string {
	return "garbage_collect_data_job"
}

func (j *GarbageCollectDataJob) purgeProperties(ctx context.Context, before time.Time) error {
	// NOTE: we're processing properties that are soft-deleted, but org is not
	if properties, err := j.BusinessDB.Impl().RetrieveSoftDeletedProperties(ctx, before, maxSoftDeletedProperties); (err == nil) && (len(properties) > 0) {
		ids := make([]int32, 0, len(properties))
		for _, p := range properties {
			ids = append(ids, p.Property.ID)
		}

		if err := j.TimeSeries.DeletePropertiesData(ctx, ids); err == nil {
			_ = j.BusinessDB.Impl().DeleteProperties(ctx, ids)
		}
	}

	return nil
}

func (j *GarbageCollectDataJob) purgeOrganizations(ctx context.Context, before time.Time) error {
	// NOTE: we're processing organizations that are soft-deleted, but user is not
	if organizations, err := j.BusinessDB.Impl().RetrieveSoftDeletedOrganizations(ctx, before, maxSoftDeletedOrganizations); (err == nil) && (len(organizations) > 0) {
		ids := make([]int32, 0, len(organizations))
		for _, p := range organizations {
			ids = append(ids, p.Organization.ID)
		}

		if err := j.TimeSeries.DeleteOrganizationsData(ctx, ids); err == nil {
			_ = j.BusinessDB.Impl().DeleteOrganizations(ctx, ids)
		}
	}

	return nil

}

func (j *GarbageCollectDataJob) purgeUsers(ctx context.Context, before time.Time) error {
	if users, err := j.BusinessDB.Impl().RetrieveSoftDeletedUsers(ctx, before, maxSoftDeletedUsers); (err == nil) && (len(users) > 0) {
		ids := make([]int32, 0, len(users))
		for _, p := range users {
			ids = append(ids, p.User.ID)
		}

		if err := j.TimeSeries.DeleteUsersData(ctx, ids); err == nil {
			_ = j.BusinessDB.Impl().DeleteUsers(ctx, ids)
		}
	}

	return nil

}

func (j *GarbageCollectDataJob) RunOnce(ctx context.Context) error {
	before := time.Now().UTC().Add(-j.Age)
	if err := j.purgeProperties(ctx, before); err != nil {
		return err
	}

	if err := j.purgeOrganizations(ctx, before); err != nil {
		return err
	}

	if err := j.purgeUsers(ctx, before); err != nil {
		return err
	}

	return nil
}

type CleanupExpiredTrialUsersJob struct {
	Age         time.Duration
	BusinessDB  db.Implementor
	PlanService billing.PlanService
	ChunkSize   int
}

func (j *CleanupExpiredTrialUsersJob) Interval() time.Duration {
	return 12 * time.Hour
}

func (j *CleanupExpiredTrialUsersJob) Jitter() time.Duration {
	return 6 * time.Hour
}

func (CleanupExpiredTrialUsersJob) Name() string {
	return "cleanup_expired_trial_users_job"
}

func (j *CleanupExpiredTrialUsersJob) RunOnce(ctx context.Context) error {
	expiredTo := time.Now().Add(-j.Age)
	expiredFrom := expiredTo.AddDate(0, -6 /*months*/, 0)
	users, err := j.BusinessDB.Impl().RetrieveUsersWithExpiredTrials(ctx, expiredFrom, expiredTo, j.PlanService.TrialStatus(), int32(j.ChunkSize))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve users with expired trials", common.ErrAttr(err))
		return err
	}

	b := &backoff.Backoff{
		Min:    200 * time.Millisecond,
		Max:    1 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	for i, u := range users {
		if (i > 0) && (err == nil) {
			time.Sleep(b.Duration())
		}

		err = j.BusinessDB.Impl().SoftDeleteUser(ctx, u.ID)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to soft-delete user", "userID", u.ID, common.ErrAttr(err))
		}
	}

	return nil
}
