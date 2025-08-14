package email

import "strings"

func Templates() map[string]string {
	return map[string]string{
		WelcomeTemplateName:          WelcomeHTMLTemplate,
		TwoFactorTemplateName:        TwoFactorHTMLTemplate,
		APIKeyExpirationTemplateName: APIKeyExpirationHTML,
		APIKeyExpiredTemplateName:    APIKeyExpiredHTML,
	}
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
