package maintenance

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
)

type mutexPeriodicJob struct {
	job common.PeriodicJob
	mux *sync.Mutex
}

var _ common.PeriodicJob = (*mutexPeriodicJob)(nil)

func (j *mutexPeriodicJob) Interval() time.Duration { return j.job.Interval() }
func (j *mutexPeriodicJob) Jitter() time.Duration   { return j.job.Jitter() }
func (j *mutexPeriodicJob) Name() string            { return j.job.Name() }

func (j *mutexPeriodicJob) RunOnce(ctx context.Context) error {
	slog.DebugContext(ctx, "About to acquire maintenance job mutex", "job", j.Name())

	j.mux.Lock()
	defer j.mux.Unlock()

	return j.job.RunOnce(ctx)
}

type mutexOneOffJob struct {
	job common.OneOffJob
	mux *sync.Mutex
}

var _ common.OneOffJob = (*mutexOneOffJob)(nil)

func (j *mutexOneOffJob) Name() string                { return j.job.Name() }
func (j *mutexOneOffJob) InitialPause() time.Duration { return j.job.InitialPause() }

func (j *mutexOneOffJob) RunOnce(ctx context.Context) error {
	slog.DebugContext(ctx, "About to acquire maintenance job mutex", "job", j.Name())

	j.mux.Lock()
	defer j.mux.Unlock()

	return j.job.RunOnce(ctx)
}
