package portal

import (
	"context"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/billing"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
)

type Jobs interface {
	OnboardUser(user *dbgen.User, plan billing.Plan) common.OneOffJob
	OffboardUser(user *dbgen.User) common.OneOffJob
}

func (s *Server) OnboardUser(user *dbgen.User, plan billing.Plan) common.OneOffJob {
	return &onboardUserJob{user: user, mailer: s.Mailer}
}

func (s *Server) OffboardUser(user *dbgen.User) common.OneOffJob {
	return &common.StubOneOffJob{}
}

type onboardUserJob struct {
	user   *dbgen.User
	mailer common.Mailer
}

func (j *onboardUserJob) Name() string {
	return "OnboardUser"
}

func (j *onboardUserJob) InitialPause() time.Duration {
	return 0
}

func (j *onboardUserJob) RunOnce(ctx context.Context) error {
	return j.mailer.SendWelcome(ctx, j.user.Email, common.GuessFirstName(j.user.Name))
}
