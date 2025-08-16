package email

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
)

type PortalMailer struct {
	Mailer                Sender
	CDNURL                string
	PortalURL             string
	EmailFrom             common.ConfigItem
	AdminEmail            common.ConfigItem
	ReplyToEmail          common.ConfigItem
	twofactorHTMLTemplate *template.Template
	twofactorTextTemplate *template.Template
	welcomeHTMLTemplate   *template.Template
	welcomeTextTemplate   *template.Template
}

func NewPortalMailer(cdnURL, portalURL string, mailer Sender, cfg common.ConfigStore) *PortalMailer {
	return &PortalMailer{
		Mailer:       mailer,
		EmailFrom:    cfg.Get(common.EmailFromKey),
		AdminEmail:   cfg.Get(common.AdminEmailKey),
		ReplyToEmail: cfg.Get(common.ReplyToEmailKey),
		CDNURL:       strings.TrimSuffix(cdnURL, "/"),
		PortalURL:    strings.TrimSuffix(portalURL, "/"),
	}
}

func (pm *PortalMailer) SetWelcomeEmail(tpl *common.EmailTemplate) {
	pm.welcomeHTMLTemplate = template.Must(template.New("HtmlBody").Parse(tpl.ContentHTML()))
	pm.welcomeTextTemplate = template.Must(template.New("TextBody").Parse(tpl.ContentText()))
}

func (pm *PortalMailer) SetTwoFactorEmail(tpl *common.EmailTemplate) {
	pm.twofactorHTMLTemplate = template.Must(template.New("HtmlBody").Parse(tpl.ContentHTML()))
	pm.twofactorTextTemplate = template.Must(template.New("TextBody").Parse(tpl.ContentText()))
}

var _ common.Mailer = (*PortalMailer)(nil)

func (pm *PortalMailer) SendTwoFactor(ctx context.Context, email string, code int) error {
	if len(email) == 0 {
		return errInvalidEmail
	}

	data := struct {
		Code        string
		PortalURL   string
		CurrentYear int
		CDNURL      string
	}{
		Code:        fmt.Sprintf("%06d", code),
		CDNURL:      pm.CDNURL,
		PortalURL:   pm.PortalURL,
		CurrentYear: time.Now().Year(),
	}

	var htmlBodyTpl bytes.Buffer
	if pm.twofactorHTMLTemplate != nil {
		if err := pm.twofactorHTMLTemplate.Execute(&htmlBodyTpl, data); err != nil {
			slog.ErrorContext(ctx, "Failed to execute HTML template", common.ErrAttr(err))
			return err
		}
	}

	var textBodyTpl bytes.Buffer
	if pm.twofactorTextTemplate != nil {
		if err := pm.twofactorTextTemplate.Execute(&textBodyTpl, data); err != nil {
			slog.ErrorContext(ctx, "Failed to execute Text template", common.ErrAttr(err))
			return err
		}
	}

	msg := &Message{
		HTMLBody:  htmlBodyTpl.String(),
		TextBody:  textBodyTpl.String(),
		Subject:   fmt.Sprintf("[%s] Your verification code is %v", common.PrivateCaptcha, data.Code),
		EmailTo:   email,
		EmailFrom: pm.EmailFrom.Value(),
		NameFrom:  common.PrivateCaptcha,
	}

	clog := slog.With("email", email, "code", data.Code)

	if err := pm.Mailer.SendEmail(ctx, msg); err != nil {
		level := slog.LevelError

		if email == pm.AdminEmail.Value() {
			level = slog.LevelWarn
			err = nil
		}

		clog.Log(ctx, level, "Failed to send two factor code", common.ErrAttr(err))

		return err
	}

	clog.InfoContext(ctx, "Sent two factor code")

	return nil
}

func (pm *PortalMailer) SendWelcome(ctx context.Context, email, name string) error {
	data := struct {
		PortalURL   string
		CurrentYear int
		CDNURL      string
		UserName    string
	}{
		CDNURL:      pm.CDNURL,
		PortalURL:   pm.PortalURL,
		CurrentYear: time.Now().Year(),
		UserName:    name,
	}

	var htmlBodyTpl bytes.Buffer
	if pm.welcomeHTMLTemplate != nil {
		if err := pm.welcomeHTMLTemplate.Execute(&htmlBodyTpl, data); err != nil {
			slog.ErrorContext(ctx, "Failed to execute HTML template", common.ErrAttr(err))
			return err
		}
	}

	var textBodyTpl bytes.Buffer
	if pm.welcomeTextTemplate != nil {
		if err := pm.welcomeTextTemplate.Execute(&textBodyTpl, data); err != nil {
			slog.ErrorContext(ctx, "Failed to execute Text template", common.ErrAttr(err))
			return err
		}
	}

	msg := &Message{
		HTMLBody:  htmlBodyTpl.String(),
		TextBody:  textBodyTpl.String(),
		Subject:   "Welcome to Private Captcha",
		EmailTo:   email,
		EmailFrom: pm.EmailFrom.Value(),
		NameFrom:  common.PrivateCaptcha,
		ReplyTo:   pm.ReplyToEmail.Value(),
	}

	if err := pm.Mailer.SendEmail(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "Failed to send welcome email", common.ErrAttr(err))

		return err
	}

	slog.InfoContext(ctx, "Sent welcome email", "email", email)

	return nil
}
