package portal

import (
	"testing"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/billing"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
)

func TestGetRegisterReturnsViewModel(t *testing.T) {
	s := &Server{
		APIURL: "/api",
		XSRF: &common.XSRFMiddleware{
			Key:     "key",
			Timeout: time.Hour,
		},
		Stage: common.StageTest,
	}
	s.canRegister.Store(true)

	vm, err := s.getRegister(nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if vm.View != loginTemplate {
		t.Fatalf("expected view %s, got %s", loginTemplate, vm.View)
	}

	renderCtx, ok := vm.Model.(*loginRenderContext)
	if !ok {
		t.Fatalf("unexpected model type %T", vm.Model)
	}

	if !renderCtx.IsRegister {
		t.Fatalf("expected register flag to be set")
	}

	if renderCtx.CaptchaRenderContext.CaptchaSitekey != db.PortalRegisterSitekey {
		t.Fatalf("unexpected captcha sitekey %s", renderCtx.CaptchaRenderContext.CaptchaSitekey)
	}

	if renderCtx.Token == "" {
		t.Fatalf("expected csrf token to be set")
	}
}

func TestIsUserNameValid(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"John Doe", true},
		{"O'Connor-Jr", true},
		{"", false},
		{"John123", false},
		{"Jane_Doe", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := isUserNameValid(tc.name); got != tc.want {
				t.Fatalf("isUserNameValid(%q)=%v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestCreateInternalTrialUsesPlanDefaults(t *testing.T) {
	plan := billing.NewPlanService(nil).GetInternalTrialPlan()
	params := createInternalTrial(plan, "trialing")

	if params.ExternalProductID != plan.ProductID() {
		t.Fatalf("expected product id %s, got %s", plan.ProductID(), params.ExternalProductID)
	}

	if params.Status != "trialing" {
		t.Fatalf("unexpected status %s", params.Status)
	}

	if params.TrialEndsAt.Time.IsZero() {
		t.Fatalf("expected trial end to be set")
	}

	if params.NextBilledAt.Valid {
		t.Fatalf("expected next billed at to be empty")
	}
}
