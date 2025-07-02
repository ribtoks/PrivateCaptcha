//go:build !enterprise

package maintenance

import (
	"context"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
)

func NewCheckLicenseJob(db.Implementor, common.ConfigStore) common.PeriodicJob {
	return &checkLicenseNoopJob{}
}

type checkLicenseNoopJob struct {
}

func (j *checkLicenseNoopJob) RunOnce(ctx context.Context) error {
	return nil
}

func (j *checkLicenseNoopJob) Interval() time.Duration {
	return 365 * 24 * time.Hour
}

func (j *checkLicenseNoopJob) Jitter() time.Duration {
	return 1
}

func (j *checkLicenseNoopJob) Name() string {
	return "CheckLicenseStubJob"
}
