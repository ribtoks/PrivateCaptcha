package email

import (
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
)

var (
	templates = []*common.EmailTemplate{
		APIKeyExirationTemplate,
		APIKeyExpiredTemplate,
		WelcomeEmailTemplate,
		TwoFactorEmailTemplate,
		OrgInvitationTemplate,
	}
)

func Templates() []*common.EmailTemplate {
	return templates
}
