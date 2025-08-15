package email

import (
	"fmt"
	"testing"
)

func TestCanBeHTML(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		text     string
		expected bool
	}{
		{welcomeHTMLTemplate, true},
		{twoFactorHTMLTemplate, true},
		{welcomeTextTemplate, false},
		{twoFactorTextTemplate, false},
		{apiKeyExpirationHTMLTemplate, true},
		{apiKeyExpiredHTMLTemplate, true},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("canBeHTML_%v", i), func(t *testing.T) {
			if actual := CanBeHTML(tc.text); actual != tc.expected {
				t.Errorf("Actual isHTML (%v) is different from expected (%v)", actual, tc.expected)
			}
		})
	}
}
