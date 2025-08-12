package maintenance

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type stubOneOffJob struct {
	executed int32
}

func (j *stubOneOffJob) Name() string {
	return "stubOneOffJob"
}

func (j *stubOneOffJob) InitialPause() time.Duration {
	return 0
}

func (j *stubOneOffJob) RunOnce(ctx context.Context) error {
	atomic.StoreInt32(&j.executed, 1)
	return nil
}

func (j *stubOneOffJob) wasExecuted() bool {
	return atomic.LoadInt32(&j.executed) == 1
}

type stubPeriodicJob struct {
	interval time.Duration
	jitter   time.Duration
	executed int32
}

func (j *stubPeriodicJob) Name() string {
	return "stubPeriodicJob"
}

func (j *stubPeriodicJob) Interval() time.Duration {
	return j.interval
}

func (j *stubPeriodicJob) Jitter() time.Duration {
	return 1
}

func (j *stubPeriodicJob) RunOnce(ctx context.Context) error {
	atomic.StoreInt32(&j.executed, 1)
	return nil
}

func (j *stubPeriodicJob) wasExecuted() bool {
	return atomic.LoadInt32(&j.executed) == 1
}

func TestOneOffJobExecution(t *testing.T) {
	jobsManager := NewJobs(nil)
	defer jobsManager.Shutdown()

	stubJob := &stubOneOffJob{}

	jobsManager.AddOneOff(stubJob)

	jobsManager.Run()

	time.Sleep(50 * time.Millisecond)

	if !stubJob.wasExecuted() {
		t.Error("OneOffJob was not executed")
	}
}

func TestPeriodicJobExecution(t *testing.T) {
	jobsManager := NewJobs(nil)
	defer jobsManager.Shutdown()

	stubJob := &stubPeriodicJob{
		interval: 10 * time.Millisecond,
	}

	jobsManager.Add(stubJob)

	jobsManager.Run()

	time.Sleep(stubJob.interval * 10)

	if !stubJob.wasExecuted() {
		t.Error("PeriodicJob was not executed")
	}
}
