package email

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestEmailTemplates(t *testing.T) {
	data := struct {
		Code               int
		PortalURL          string
		CurrentYear        int
		CDNURL             string
		Message            string
		TicketID           string
		APIKeyName         string
		APIKeyPrefix       string
		ExpireDays         int
		APIKeySettingsPath string
		UserName           string
	}{
		UserName:           "John Doe",
		Code:               123456,
		CDNURL:             "https://cdn.privatecaptcha.com",
		PortalURL:          "https://portal.privatecaptcha.com",
		CurrentYear:        time.Now().Year(),
		Message:            "This is a support request message. Nothing works!",
		TicketID:           "qwerty12345",
		APIKeyName:         "My API Key",
		APIKeyPrefix:       "abcde",
		ExpireDays:         7,
		APIKeySettingsPath: "settings?tab=apikeys",
	}

	for _, tpl := range templates {
		t.Run(fmt.Sprintf("emailTemplate_%v", tpl.Name()), func(t *testing.T) {
			ctx := context.TODO()

			if _, err := tpl.RenderHTML(ctx, data); err != nil {
				t.Fatal(err)
			}

			if _, err := tpl.RenderText(ctx, data); err != nil {
				t.Fatal(err)
			}
		})
	}
}
