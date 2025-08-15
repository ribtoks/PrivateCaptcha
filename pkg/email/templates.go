package email

import (
	"strings"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
)

var (
	templates = []*common.EmailTemplate{
		APIKeyExirationTemplate,
		APIKeyExpiredTemplate,
		WelcomeEmailTemplate,
		TwoFactorEmailTemplate,
	}
)

func Templates() []*common.EmailTemplate {
	return templates
}

func CanBeHTML(s string) bool {
	suffix := s[max(0, len(s)-100):]
	if !strings.Contains(suffix, "</body>") ||
		!strings.Contains(suffix, "</html>") ||
		!strings.Contains(suffix, "</") {
		return false
	}

	prefix := s[:min(len(s), 200)]
	if !strings.Contains(prefix, "<head>") ||
		!strings.Contains(prefix, "<html") {
		return false
	}

	return true
}
