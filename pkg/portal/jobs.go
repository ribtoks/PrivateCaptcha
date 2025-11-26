package portal

import (
	"context"
	"log/slog"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/billing"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/session"
)

type Jobs interface {
	OnboardUser(user *dbgen.User, plan billing.Plan) common.OneOffJob
	OffboardUser(user *dbgen.User) common.OneOffJob
	LoginUser(sess *session.Session) common.OneOffJob
}

func (s *Server) OnboardUser(user *dbgen.User, plan billing.Plan) common.OneOffJob {
	return &onboardUserJob{user: user, mailer: s.Mailer}
}

func (s *Server) OffboardUser(user *dbgen.User) common.OneOffJob {
	return &common.StubOneOffJob{}
}

func (s *Server) LoginUser(sess *session.Session) common.OneOffJob {
	return &loginUserJob{
		sess:  sess,
		store: s.Store,
	}
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

func (j *onboardUserJob) NewParams() any {
	return struct{}{}
}

func (j *onboardUserJob) RunOnce(ctx context.Context, params any) error {
	return j.mailer.SendWelcome(ctx, j.user.Email, common.GuessFirstName(j.user.Name))
}

type loginUserJob struct {
	sess  *session.Session
	store db.Implementor
}

func (j *loginUserJob) Name() string {
	return "LoginUser"
}
func (j *loginUserJob) InitialPause() time.Duration {
	return 0
}
func (loginuserjob *loginUserJob) NewParams() any {
	return struct{}{}
}
func (j *loginUserJob) RunOnce(ctx context.Context, params any) error {
	userID, hasUserID := j.sess.Get(ctx, session.KeyUserID).(int32)
	if hasUserID {
		j.store.AuditLog().RecordEvent(ctx, newUserAuthAuditLogEvent(userID, common.AuditLogActionLogin))

		slog.DebugContext(ctx, "Fetching system notification for user", "userID", userID)
		if n, err := j.store.Impl().RetrieveSystemUserNotification(ctx, time.Now().UTC(), userID); err == nil {
			_ = j.sess.Set(session.KeyNotificationID, n.ID)
		}
	} else {
		slog.ErrorContext(ctx, "UserID not found in session")
	}

	return nil
}
