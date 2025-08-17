package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
)

func NewJobs(store db.Implementor) *jobs {
	return &jobs{
		store:        store,
		periodicJobs: make([]common.PeriodicJob, 0),
		oneOffJobs:   make([]common.OneOffJob, 0),
	}
}

type jobs struct {
	store             db.Implementor
	periodicJobs      []common.PeriodicJob
	oneOffJobs        []common.OneOffJob
	maintenanceCancel context.CancelFunc
	maintenanceCtx    context.Context
	mux               sync.Mutex
}

// Implicit logic is that lockDuration is the actual job Interval, but it is defined by the SQL lock.
// Job's Interval() is much smaller only for the purpose of "retrying" if the previous job execution failed
func (j *jobs) AddLocked(lockDuration time.Duration, job common.PeriodicJob) {
	if interval := job.Interval(); interval >= lockDuration {
		slog.Error("Periodic job interval should be less than lock duration", "job", job.Name(), "lock", lockDuration.String(), "interval", interval.String())
	}

	j.periodicJobs = append(j.periodicJobs, &UniquePeriodicJob{
		Job:          job,
		Store:        j.store,
		LockDuration: lockDuration,
	})
}

func (j *jobs) Add(job common.PeriodicJob) {
	j.periodicJobs = append(j.periodicJobs, job)
}

func (j *jobs) AddOneOff(job common.OneOffJob) {
	j.oneOffJobs = append(j.oneOffJobs, job)
}

func (j *jobs) Run() {
	j.maintenanceCtx, j.maintenanceCancel = context.WithCancel(
		context.WithValue(context.Background(), common.TraceIDContextKey, "maintenance"))

	slog.DebugContext(j.maintenanceCtx, "Starting maintenance jobs", "periodic", len(j.periodicJobs), "oneoff", len(j.oneOffJobs))

	// NOTE: we run jobs mutually exclusive to preserve resources for main server (those are _maintenance_ jobs anyways)
	// NOTE 2: this does not apply for on-demand ones below

	for _, job := range j.periodicJobs {
		go common.RunPeriodicJob(j.maintenanceCtx, &mutexPeriodicJob{job: job, mux: &j.mux})
	}

	for _, job := range j.oneOffJobs {
		go common.RunOneOffJob(j.maintenanceCtx, &mutexOneOffJob{job: job, mux: &j.mux})
	}
}

func (j *jobs) Setup(mux *http.ServeMux) {
	mux.Handle(http.MethodPost+" /maintenance/periodic/{job}", common.Recovered(http.HandlerFunc(j.handlePeriodicJob)))
	mux.Handle(http.MethodPost+" /maintenance/oneoff/{job}", common.Recovered(http.HandlerFunc(j.handleOneoffJob)))
}

func (j *jobs) handlePeriodicJob(w http.ResponseWriter, r *http.Request) {
	jobName, err := common.StrPathArg(r, "job")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	slog.DebugContext(ctx, "Handling on-demand periodic job launch", "job", jobName)
	found := false

	for _, job := range j.periodicJobs {
		if job.Name() == jobName {
			go func() {
				_ = common.RunPeriodicJobOnce(common.CopyTraceID(ctx, context.Background()), job)
			}()
			found = true
			break
		}
	}

	if !found {
		http.Error(w, fmt.Sprintf("job %v not found", jobName), http.StatusBadRequest)
		return
	}

	_, _ = w.Write([]byte("started"))
}

func (j *jobs) handleOneoffJob(w http.ResponseWriter, r *http.Request) {
	jobName, err := common.StrPathArg(r, "job")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	slog.DebugContext(ctx, "Handling on-demand one-off job launch", "job", jobName)
	found := false

	for _, job := range j.oneOffJobs {
		if job.Name() == jobName {
			go common.RunOneOffJob(common.CopyTraceID(ctx, context.Background()), job)
			found = true
			break
		}
	}

	if !found {
		http.Error(w, fmt.Sprintf("job %v not found", jobName), http.StatusBadRequest)
		return
	}

	_, _ = w.Write([]byte("started"))
}

func (j *jobs) Shutdown() {
	slog.Debug("Shutting down maintenance jobs")

	if j.maintenanceCancel != nil {
		j.maintenanceCancel()
	}
}
