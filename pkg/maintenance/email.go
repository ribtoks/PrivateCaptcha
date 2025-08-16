package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"text/template"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
	"github.com/jpillora/backoff"
)

type RegisterEmailTemplatesJob struct {
	Templates []*common.EmailTemplate
	Store     db.Implementor
}

var _ common.OneOffJob = (*RegisterEmailTemplatesJob)(nil)

func (j *RegisterEmailTemplatesJob) Name() string {
	return "register_email_templates_job"
}
func (j *RegisterEmailTemplatesJob) InitialPause() time.Duration {
	return 20 * time.Second
}

func (j *RegisterEmailTemplatesJob) RunOnce(ctx context.Context) error {
	var anyError error

	for _, tpl := range j.Templates {
		if _, err := j.Store.Impl().CreateNotificationTemplate(ctx, tpl.Name(), tpl.ContentHTML(), tpl.ContentText(), tpl.Hash()); err != nil {
			slog.ErrorContext(ctx, "Failed to upsert notification template", "name", tpl.Name(), common.ErrAttr(err))
			anyError = err
		}
	}

	return anyError
}

type UserEmailNotificationsJob struct {
	// this is the "actual" interval since we will be running as a DB-locked distributed job
	RunInterval  time.Duration
	Store        db.Implementor
	Templates    []*common.EmailTemplate
	Sender       email.Sender
	ChunkSize    int
	EmailFrom    common.ConfigItem
	ReplyToEmail common.ConfigItem
	CDNURL       string
	PortalURL    string
}

var _ common.PeriodicJob = (*UserEmailNotificationsJob)(nil)

func (j *UserEmailNotificationsJob) Interval() time.Duration {
	return 20 * time.Minute
}

func (j *UserEmailNotificationsJob) Jitter() time.Duration {
	return 10 * time.Minute
}

func (j *UserEmailNotificationsJob) Name() string {
	return "user_email_notifications_job"
}

func groupNotificationsByTemplate(ctx context.Context, notifications []*dbgen.GetPendingUserNotificationsRow) map[string][]*dbgen.GetPendingUserNotificationsRow {
	result := make(map[string][]*dbgen.GetPendingUserNotificationsRow)

	for _, n := range notifications {
		un := &n.UserNotification
		if !un.TemplateID.Valid {
			slog.ErrorContext(ctx, "Skipping notification template with orphanned hash", "nid", un.ID)
			continue
		}

		if list, ok := result[un.TemplateID.String]; ok {
			result[un.TemplateID.String] = append(list, n)
		} else {
			result[un.TemplateID.String] = []*dbgen.GetPendingUserNotificationsRow{n}
		}
	}

	return result
}

func indexTemplates(ctx context.Context, templates []*common.EmailTemplate) map[string]*common.EmailTemplate {
	tplMap := make(map[string]*common.EmailTemplate)
	for _, tpl := range templates {
		hash := tpl.Hash()
		if _, ok := tplMap[hash]; ok {
			slog.ErrorContext(ctx, "Found two templates with the same hash", "hash", hash, "name", tpl.Name())
			continue
		}
		tplMap[hash] = tpl
	}
	return tplMap
}

type preparedNotificationTemplate struct {
	htmlTemplate *template.Template
	textTemplate *template.Template
	name         string
}

func (j *UserEmailNotificationsJob) retrieveTemplate(ctx context.Context,
	templates map[string]*common.EmailTemplate,
	templateHash string) (*preparedNotificationTemplate, error) {
	hlog := slog.With("hash", templateHash)
	var contentHTML string
	var contentText string
	var name string
	itpl, ok := templates[templateHash]
	if ok {
		contentHTML, contentText = itpl.ContentHTML(), itpl.ContentText()
		name = itpl.Name()
	} else {
		hlog.WarnContext(ctx, "Template is not found locally")
		if dbTemplate, err := j.Store.Impl().RetrieveNotificationTemplate(ctx, templateHash); err == nil {
			contentHTML, contentText = dbTemplate.ContentHtml, dbTemplate.ContentText
			name = dbTemplate.Name
		} else {
			hlog.ErrorContext(ctx, "Failed to retrieve template from DB", common.ErrAttr(err))
			return nil, err
		}
	}

	nt := &preparedNotificationTemplate{name: name}

	if len(contentHTML) > 0 {
		if tplHTML, err := template.New("NotificationHTML").Parse(contentHTML); err != nil {
			hlog.ErrorContext(ctx, "Failed to parse HTML template", "name", name, common.ErrAttr(err))
			return nil, err
		} else {
			nt.htmlTemplate = tplHTML
		}
	}

	if len(contentText) > 0 {
		if tplText, err := template.New("NotificationText").Parse(contentText); err != nil {
			hlog.ErrorContext(ctx, "Failed to parse text template", "name", name, common.ErrAttr(err))
			return nil, err
		} else {
			nt.textTemplate = tplText
		}
	}

	hlog.InfoContext(ctx, "Parsed templates", "name", name)

	return nt, nil
}

func (j *UserEmailNotificationsJob) RunOnce(ctx context.Context) error {
	// just for safety, we fetch overlapping segments, but it will be filtered out on the way
	since := time.Now().UTC().Add(-(j.RunInterval + j.Interval() + j.Jitter()))
	notifications, err := j.Store.Impl().RetrievePendingUserNotifications(ctx, since, j.ChunkSize)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve pending user notifications", common.ErrAttr(err))
		return err
	}

	if len(notifications) == 0 {
		slog.DebugContext(ctx, "No pending notifications", "since", since)
		return nil
	}

	templates := indexTemplates(ctx, j.Templates)

	b := &backoff.Backoff{
		Min:    50 * time.Millisecond,
		Max:    1 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	groups := groupNotificationsByTemplate(ctx, notifications)
	for tplHash, nn := range groups {
		if len(nn) == 0 {
			slog.WarnContext(ctx, "Skipping empty notifications for template", "hash", tplHash)
			continue
		}

		if tpl, err := j.retrieveTemplate(ctx, templates, tplHash); err == nil {
			_ = j.processNotificationsChunk(ctx, tpl, nn, b)
		} else {
			slog.ErrorContext(ctx, "Failed to get notifications template", common.ErrAttr(err))
		}
	}

	return nil
}

func isValidUserNotification(un *dbgen.UserNotification) bool {
	return len(un.Subject) > 0 &&
		un.ScheduledAt.Valid &&
		un.TemplateID.Valid
}

func (j *UserEmailNotificationsJob) processNotificationsChunk(ctx context.Context,
	tpl *preparedNotificationTemplate,
	notifications []*dbgen.GetPendingUserNotificationsRow,
	b *backoff.Backoff) error {
	emailFrom := j.EmailFrom.Value()
	replyToEmail := j.ReplyToEmail.Value()
	sentNotificationIDs := make([]int32, 0, len(notifications))
	lastSentCount := 0

	for _, n := range notifications {
		if currSentCount := len(sentNotificationIDs); currSentCount > lastSentCount {
			// backoff a little not to overwhelm transactional email provider
			time.Sleep(b.Duration())
			lastSentCount = currSentCount
		}

		un := &n.UserNotification
		nlog := slog.With("nid", un.ID, "template_id", un.TemplateID.String, "template_name", tpl.name)
		if !isValidUserNotification(un) {
			nlog.WarnContext(ctx, "Skipping invalid user notification")
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(un.Payload, &data); err != nil {
			nlog.ErrorContext(ctx, "Failed to parse notification context", common.ErrAttr(err))
			continue
		}
		// "common" context
		data["CDNURL"] = j.CDNURL
		data["PortalURL"] = j.PortalURL
		data["CurrentYear"] = time.Now().Year()

		var htmlBodyTpl bytes.Buffer
		if tpl.htmlTemplate != nil {
			if err := tpl.htmlTemplate.Execute(&htmlBodyTpl, data); err != nil {
				nlog.ErrorContext(ctx, "Failed to execute HTML template with notification context", common.ErrAttr(err))
				continue
			}
		}

		var textBodyTpl bytes.Buffer
		if tpl.textTemplate != nil {
			if err := tpl.textTemplate.Execute(&textBodyTpl, data); err != nil {
				nlog.ErrorContext(ctx, "Failed to execute Text template with notification context", common.ErrAttr(err))
				continue
			}
		}

		msg := &email.Message{
			Subject:   un.Subject,
			EmailTo:   n.Email,
			EmailFrom: emailFrom,
			NameFrom:  common.PrivateCaptchaTeam,
			ReplyTo:   replyToEmail,
			HTMLBody:  htmlBodyTpl.String(),
			TextBody:  textBodyTpl.String(),
		}

		if err := j.Sender.SendEmail(ctx, msg); err != nil {
			nlog.ErrorContext(ctx, "Failed to send notification email", common.ErrAttr(err))
			continue
		}

		nlog.DebugContext(ctx, "Processed user notification")

		sentNotificationIDs = append(sentNotificationIDs, un.ID)
	}

	if err := j.Store.Impl().MarkUserNotificationsSent(ctx, sentNotificationIDs, time.Now().UTC()); err != nil {
		slog.ErrorContext(ctx, "Failed to mark notifications sent", common.ErrAttr(err))
	}

	return nil
}

type CleanupUserNotificationsJob struct {
	Store db.Implementor
}

func (j *CleanupUserNotificationsJob) RunOnce(ctx context.Context) error {
	var anyError error

	tnow := time.Now().UTC()

	if err := j.Store.Impl().DeleteSentUserNotifications(ctx, tnow.AddDate(0, -6 /*months*/, 0)); err != nil {
		slog.ErrorContext(ctx, "Failed to delete sent user notifications", common.ErrAttr(err))
		anyError = err
	}

	if err := j.Store.Impl().DeleteUnsentUserNotifications(ctx, tnow.AddDate(0, -6 /*months*/, 0)); err != nil {
		slog.ErrorContext(ctx, "Failed to delete UNsent user notifications", common.ErrAttr(err))
		anyError = err
	}

	// we delete notification templates only if there're no dangling references in
	// user_notifications table so the date should be smaller than the other two
	templateUpdatedBefore := tnow.AddDate(0, -7 /*months*/, 0)
	notifDeliveredBefore := tnow.AddDate(0, -7 /*months*/, 0)
	if err := j.Store.Impl().DeleteUnusedNotificationTemplates(ctx, notifDeliveredBefore, templateUpdatedBefore); err != nil {
		slog.ErrorContext(ctx, "Failed to delete unused notification templates", common.ErrAttr(err))
		anyError = err
	}

	return anyError
}

func (j *CleanupUserNotificationsJob) Interval() time.Duration {
	return 3 * time.Hour
}

func (j *CleanupUserNotificationsJob) Jitter() time.Duration {
	return 1 * time.Hour
}
func (j *CleanupUserNotificationsJob) Name() string {
	return "cleanup_user_notifications_job"
}
