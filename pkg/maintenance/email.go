package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"text/template"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/config"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
	"github.com/jpillora/backoff"
)

type RegisterEmailTemplatesJob struct {
	Templates map[string]string
	Store     db.Implementor
}

var _ common.OneOffJob = (*RegisterEmailTemplatesJob)(nil)

func (j *RegisterEmailTemplatesJob) Name() string {
	return "RegisterEmailTemplatesJob"
}
func (j *RegisterEmailTemplatesJob) InitialPause() time.Duration {
	return 20 * time.Second
}

func (j *RegisterEmailTemplatesJob) RunOnce(ctx context.Context) error {
	var anyError error

	for name, content := range j.Templates {
		if _, err := j.Store.Impl().CreateNotificationTemplate(ctx, name, content); err != nil {
			slog.ErrorContext(ctx, "Failed to upsert notification template", "name", name, common.ErrAttr(err))
			anyError = err
		}
	}

	return anyError
}

type UserEmailNotificationsJob struct {
	// this is the "actual" interval since we will be running as a DB-locked distributed job
	RunInterval  time.Duration
	Store        db.Implementor
	Templates    map[string]string
	Sender       email.Sender
	ChunkSize    common.ConfigItem
	EmailFrom    common.ConfigItem
	ReplyToEmail common.ConfigItem
}

var _ common.PeriodicJob = (*UserEmailNotificationsJob)(nil)

func (j *UserEmailNotificationsJob) Interval() time.Duration {
	return 20 * time.Minute
}

func (j *UserEmailNotificationsJob) Jitter() time.Duration {
	return 10 * time.Minute
}

func (j *UserEmailNotificationsJob) Name() string {
	return "UserEmailNotificationsJob"
}

func groupNotificationsByTemplate(ctx context.Context, notifications []*dbgen.GetPendingUserNotificationsRow) map[string][]*dbgen.GetPendingUserNotificationsRow {
	result := make(map[string][]*dbgen.GetPendingUserNotificationsRow)

	for _, n := range notifications {
		un := &n.UserNotification
		if !un.TemplateHash.Valid {
			slog.ErrorContext(ctx, "Skipping notification template with orphanned hash", "nid", un.ID)
			continue
		}

		if list, ok := result[un.TemplateHash.String]; ok {
			result[un.TemplateHash.String] = append(list, n)
		} else {
			result[un.TemplateHash.String] = []*dbgen.GetPendingUserNotificationsRow{n}
		}
	}

	return result
}

type indexedNotificationTemplate struct {
	name    string
	hash    string
	content string
}

func indexTemplates(ctx context.Context, nameToContentTplMap map[string]string) map[string]*indexedNotificationTemplate {
	templates := make(map[string]*indexedNotificationTemplate)
	for name, content := range nameToContentTplMap {
		hash := db.EmailTemplateHash(content)
		if _, ok := templates[hash]; ok {
			slog.ErrorContext(ctx, "Found two templates with the same hash", "hash", hash, "name", name)
			continue
		}
		templates[hash] = &indexedNotificationTemplate{
			name:    name,
			hash:    hash,
			content: content,
		}
	}
	return templates
}

type preparedNotificationTemplate struct {
	tpl    *template.Template
	name   string
	isHTML bool
}

func (j *UserEmailNotificationsJob) retrieveTemplate(ctx context.Context,
	templates map[string]*indexedNotificationTemplate,
	templateHash string) (*preparedNotificationTemplate, error) {
	hlog := slog.With("hash", templateHash)
	var content string
	var name string
	itpl, ok := templates[templateHash]
	if ok {
		content = itpl.content
		name = itpl.name
	} else {
		hlog.WarnContext(ctx, "Template is not found locally")
		if dbTemplate, err := j.Store.Impl().RetrieveNotificationTemplate(ctx, templateHash); err == nil {
			content = dbTemplate.Content
			name = dbTemplate.Name
		} else {
			hlog.ErrorContext(ctx, "Failed to retrieve template from DB", common.ErrAttr(err))
			return nil, err
		}
	}

	tpl, err := template.New("Notification").Parse(content)
	if err != nil {
		hlog.ErrorContext(ctx, "Failed to parse HTML template", "name", name, common.ErrAttr(err))
		return nil, err
	}

	isHTML := email.CanBeHTML(content)

	hlog.InfoContext(ctx, "Parsed template", "name", name, "length", len(content), "html", isHTML)

	return &preparedNotificationTemplate{
		tpl:    tpl,
		name:   name,
		isHTML: isHTML,
	}, nil
}

func (j *UserEmailNotificationsJob) RunOnce(ctx context.Context) error {
	// just for safety, we fetch overlapping segments, but it will be filtered out on the way
	since := time.Now().UTC().Add(-(j.RunInterval + j.Interval() + j.Jitter()))
	chunkSize := config.AsInt(j.ChunkSize, 100 /*fallback*/)
	notifications, err := j.Store.Impl().RetrievePendingUserNotifications(ctx, since, chunkSize)
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
		un.TemplateHash.Valid
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
			currSentCount = lastSentCount
		}

		un := &n.UserNotification
		nlog := slog.With("nid", un.ID, "hash", un.TemplateHash.String, "template", tpl.name)
		if !isValidUserNotification(un) {
			nlog.WarnContext(ctx, "Skipping invalid user notification")
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(un.Payload, &data); err != nil {
			nlog.ErrorContext(ctx, "Failed to parse notification context", common.ErrAttr(err))
			continue
		}

		var bodyTpl bytes.Buffer
		if err := tpl.tpl.Execute(&bodyTpl, data); err != nil {
			nlog.ErrorContext(ctx, "Failed to execute template with notification context", common.ErrAttr(err))
			continue
		}

		msg := &email.Message{
			Subject:   un.Subject,
			EmailTo:   n.Email,
			EmailFrom: emailFrom,
			NameFrom:  common.PrivateCaptcha,
			ReplyTo:   replyToEmail,
		}

		if tpl.isHTML {
			msg.HTMLBody = bodyTpl.String()
		} else {
			msg.TextBody = bodyTpl.String()
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

	if err := j.Store.Impl().DeleteSentUserNotifications(ctx, tnow.AddDate(0, -3 /*months*/, 0)); err != nil {
		slog.ErrorContext(ctx, "Failed to delete sent user notifications", common.ErrAttr(err))
		anyError = err
	}

	if err := j.Store.Impl().DeleteUnsentUserNotifications(ctx, tnow.AddDate(0, -6 /*months*/, 0)); err != nil {
		slog.ErrorContext(ctx, "Failed to delete UNsent user notifications", common.ErrAttr(err))
		anyError = err
	}

	if err := j.Store.Impl().DeleteUnusedNotificationTemplates(ctx, tnow.AddDate(0, -6 /*months*/, 0)); err != nil {
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
	return "CleanupUserNotificationsJob"
}
