package portal

import (
	"context"
	"errors"
	"fmt"
	htmltpl "html/template"
	"log/slog"
	"strings"
	texttpl "text/template"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	emailpkg "github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
)

var (
	errInvalidEmail = errors.New("email is not valid")
)

type PortalMailer struct {
	Mailer                emailpkg.Sender
	CDNURL                string
	PortalURL             string
	EmailFrom             common.ConfigItem
	AdminEmail            common.ConfigItem
	ReplyToEmail          common.ConfigItem
	twofactorHTMLTemplate *htmltpl.Template
	twofactorTextTemplate *texttpl.Template
	welcomeHTMLTemplate   *htmltpl.Template
	welcomeTextTemplate   *texttpl.Template
}

func NewPortalMailer(cdnURL, portalURL string, mailer emailpkg.Sender, cfg common.ConfigStore) *PortalMailer {
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
	pm.welcomeHTMLTemplate, pm.welcomeTextTemplate = tpl.Parse()
}

func (pm *PortalMailer) SetTwoFactorEmail(tpl *common.EmailTemplate) {
	pm.twofactorHTMLTemplate, pm.twofactorTextTemplate = tpl.Parse()
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

	htmlBody, err := common.RenderHTMLTemplate(ctx, pm.twofactorHTMLTemplate, data)
	if err != nil {
		return err
	}

	textBody, err := common.RenderTextTemplate(ctx, pm.twofactorTextTemplate, data)
	if err != nil {
		return err
	}

	msg := &emailpkg.Message{
		HTMLBody:  htmlBody,
		TextBody:  textBody,
		Subject:   fmt.Sprintf("[%s] Your verification code is %v", common.PrivateCaptcha, data.Code),
		EmailTo:   email,
		EmailFrom: pm.EmailFrom.Value(),
		NameFrom:  common.PrivateCaptchaTeam,
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

	htmlBody, err := common.RenderHTMLTemplate(ctx, pm.welcomeHTMLTemplate, data)
	if err != nil {
		return err
	}

	textBody, err := common.RenderTextTemplate(ctx, pm.welcomeTextTemplate, data)
	if err != nil {
		return err
	}

	msg := &emailpkg.Message{
		HTMLBody:  htmlBody,
		TextBody:  textBody,
		Subject:   "Welcome to Private Captcha",
		EmailTo:   email,
		EmailFrom: pm.EmailFrom.Value(),
		NameFrom:  common.PrivateCaptchaTeam,
		ReplyTo:   pm.ReplyToEmail.Value(),
	}

	if err := pm.Mailer.SendEmail(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "Failed to send welcome email", common.ErrAttr(err))

		return err
	}

	slog.InfoContext(ctx, "Sent welcome email", "email", email)

	return nil
}
