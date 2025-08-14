package portal

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/session"
)

var (
	errEmailTemplateNotFound = errors.New("template with such name does not exist")
)

func (s *Server) createSystemNotificationContext(ctx context.Context, sess *common.Session) systemNotificationContext {
	renderCtx := systemNotificationContext{}

	if notificationID, ok := sess.Get(session.KeyNotificationID).(int32); ok {
		if notification, err := s.Store.Impl().RetrieveSystemNotification(ctx, notificationID); err == nil {
			renderCtx.Notification = notification.Message
			renderCtx.NotificationID = strconv.Itoa(int(notification.ID))
		}
	}

	return renderCtx
}

func (s *Server) dismissNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess := s.Sessions.SessionStart(w, r)

	value := r.PathValue(common.ParamID)
	id, err := strconv.Atoi(value)
	if err == nil {
		if notificationID, ok := sess.Get(session.KeyNotificationID).(int32); ok {
			if notificationID != int32(id) {
				slog.ErrorContext(ctx, "Mismatch between notification ID in session", "session", notificationID, "param", id)
			}
		}
		if derr := sess.Delete(session.KeyNotificationID); derr != nil {
			slog.ErrorContext(ctx, "Failed to dismiss notification", "id", id, common.ErrAttr(derr))
		} else {
			slog.DebugContext(ctx, "Dismissed notification", "id", id)
		}
		w.WriteHeader(http.StatusOK)
	} else {
		slog.ErrorContext(ctx, "Failed to parse notification ID", "id", value[:10], "length", len(value), common.ErrAttr(err))
		http.Error(w, "", http.StatusBadRequest)
	}
}

type NotificationScheduler struct {
	Store db.Implementor
}

var _ common.ScheduledNotifications = (*NotificationScheduler)(nil)

func (ns *NotificationScheduler) Add(ctx context.Context, n *common.ScheduledNotification) {
	_, _ = ns.AddEx(ctx, n)
}

func (ns *NotificationScheduler) AddEx(ctx context.Context, n *common.ScheduledNotification) (*dbgen.UserNotification, error) {
	templates := email.Templates()
	template, ok := templates[n.TemplateName]
	if !ok {
		slog.ErrorContext(ctx, "Notification template with such name does not exist", "name", n.TemplateName)
		return nil, errEmailTemplateNotFound
	}

	// NOTE: we don't add template to DB (again) because it should have been done with RegisterEmailTemplatesJob on startup

	templateHash := db.EmailTemplateHash(template)
	notif, err := ns.Store.Impl().CreateUserNotification(ctx, n.UserID, n.ReferenceID, n.Data, n.Subject, templateHash, n.DateTime)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to add scheduled notification", common.ErrAttr(err))
		return nil, err
	}

	return notif, nil
}

func (ns *NotificationScheduler) RemoveEx(ctx context.Context, userID int32, referenceID string) error {
	if err := ns.Store.Impl().DeletePendingUserNotification(ctx, userID, referenceID); err != nil {
		slog.ErrorContext(ctx, "Failed to delete scheduled notification", common.ErrAttr(err))
		return err
	}

	return nil
}

func (ns *NotificationScheduler) Remove(ctx context.Context, userID int32, referenceID string) {
	_ = ns.RemoveEx(ctx, userID, referenceID)
}
