package portal

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/puzzle"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/session"
	"github.com/badoux/checkmail"
)

const (
	loginStepSignInVerify = 1
	loginStepSignUpVerify = 2
	loginStepCompleted    = 3
	loginFormTemplate     = "login/form.html"
	loginTemplate         = "login/login.html"
)

var (
	errPortalPropertyNotFound = errors.New("portal property not found")
)

type loginRenderContext struct {
	CsrfRenderContext
	CaptchaRenderContext
	EmailError  string
	CanRegister bool
}

type portalPropertyOwnerSource struct {
	Store   db.Implementor
	Sitekey string
}

var _ puzzle.OwnerIDSource = (*portalPropertyOwnerSource)(nil)

func (s *portalPropertyOwnerSource) OwnerID(ctx context.Context, tnow time.Time) (int32, error) {
	property, err := s.Store.Impl().RetrievePropertyBySitekey(ctx, s.Sitekey)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to fetch login property", common.ErrAttr(err))
		return -1, errPortalPropertyNotFound
	}

	return property.OrgOwnerID.Int32, nil
}

func (s *Server) getLogin(w http.ResponseWriter, r *http.Request) (Model, string, error) {
	return &loginRenderContext{
		CsrfRenderContext: CsrfRenderContext{
			Token: s.XSRF.Token(""),
		},
		CaptchaRenderContext: s.CreateCaptchaRenderContext(db.PortalLoginSitekey),
		CanRegister:          s.canRegister.Load(),
	}, loginTemplate, nil
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to read request body", common.ErrAttr(err))
		s.RedirectError(http.StatusBadRequest, w, r)
		return
	}

	data := &loginRenderContext{
		CsrfRenderContext: CsrfRenderContext{
			Token: s.XSRF.Token(""),
		},
		CaptchaRenderContext: s.CreateCaptchaRenderContext(db.PortalLoginSitekey),
		CanRegister:          s.canRegister.Load(),
	}

	captchaSolution := r.FormValue(captchaSolutionField)
	if len(captchaSolution) == 0 {
		data.CaptchaError = "You need to solve captcha to login."
		s.render(w, r, loginFormTemplate, data)
		return
	}

	ownerSource := &portalPropertyOwnerSource{Store: s.Store, Sitekey: data.CaptchaSitekey}
	verifyResult, err := s.PuzzleEngine.Verify(ctx, []byte(captchaSolution), ownerSource, time.Now().UTC())
	if err != nil || !verifyResult.Success() {
		slog.ErrorContext(ctx, "Failed to verify captcha", "verify", verifyResult.ErrorsToStrings(), common.ErrAttr(err))
		data.CaptchaError = "Captcha verification failed"
		s.render(w, r, loginFormTemplate, data)
		return
	}

	email := strings.TrimSpace(r.FormValue(common.ParamEmail))
	if err = checkmail.ValidateFormat(email); err != nil {
		slog.WarnContext(ctx, "Failed to validate email format", common.ErrAttr(err))
		data.EmailError = "Email address is not valid."
		s.render(w, r, loginFormTemplate, data)
		return
	}

	user, err := s.Store.Impl().FindUserByEmail(ctx, email)
	if err != nil {
		slog.WarnContext(ctx, "Failed to find user by email", "email", email, common.ErrAttr(err))
		data.EmailError = "User with such email does not exist."
		s.render(w, r, loginFormTemplate, data)
		return
	}

	sess := s.Sessions.SessionStart(w, r)
	if step, ok := sess.Get(session.KeyLoginStep).(int); ok {
		if step == loginStepCompleted {
			slog.DebugContext(ctx, "User seem to be already logged in", "email", email)
			common.Redirect(s.RelURL("/"), http.StatusOK, w, r)
			return
		} else {
			slog.WarnContext(ctx, "Session present, but login not finished", "step", step, "email", email)
		}
	}

	code := twoFactorCode()

	if err := s.Mailer.SendTwoFactor(ctx, user.Email, code); err != nil {
		slog.ErrorContext(ctx, "Failed to send email message", common.ErrAttr(err))
		s.RedirectError(http.StatusInternalServerError, w, r)
		return
	}

	_ = sess.Set(session.KeyLoginStep, loginStepSignInVerify)
	_ = sess.Set(session.KeyUserEmail, user.Email)
	_ = sess.Set(session.KeyUserName, user.Name)
	_ = sess.Set(session.KeyTwoFactorCode, code)
	_ = sess.Set(session.KeyUserID, user.ID)

	common.Redirect(s.RelURL(common.TwoFactorEndpoint), http.StatusOK, w, r)
}
