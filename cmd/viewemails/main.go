package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
)

const (
	rootTemplateStart = `
<html>
<body>
<strong>Templates:</strong>
<ul>
`
	rootTemplateEnd = `</ul>
</body>
</html>`
)

var (
	templates = map[string]string{}
)

func homepage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(rootTemplateStart))

	keys := make([]string, 0, len(templates))
	for k := range templates {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		_, _ = fmt.Fprintf(w, "<li><a href=\"/%s\">%s</a></li>\n", k, k)
	}
	_, _ = w.Write([]byte(rootTemplateEnd))
}

func serveExecute(templateBody string, w http.ResponseWriter) error {
	tpl, err := template.New("HtmlBody").Parse(templateBody)
	if err != nil {
		log.Printf("Failed to parse template: %v", err)
		return err
	}

	data := struct {
		email.OrgInvitationContext
		email.APIKeyExpirationContext
		// heap of everything else
		Code        int
		PortalURL   string
		CurrentYear int
		CDNURL      string
		UserName    string
	}{
		APIKeyExpirationContext: email.APIKeyExpirationContext{
			APIKeyContext: email.APIKeyContext{
				APIKeyName:         "My API Key",
				APIKeyPrefix:       db.APIKeyPrefix + "abcd",
				APIKeySettingsPath: "settings?tab=apikeys",
			},
			ExpireDays: 7,
		},
		OrgInvitationContext: email.OrgInvitationContext{
			//UserName:      "John Doe",
			OrgName:       "My Organization",
			OrgOwnerName:  "Pat Smith",
			OrgOwnerEmail: "john.doe@example.com",
			OrgURL:        "https://portal.privatecaptcha.com/org/5",
		},
		UserName:    "John Doe",
		Code:        123456,
		CDNURL:      "https://cdn.privatecaptcha.com",
		PortalURL:   "https://portal.privatecaptcha.com",
		CurrentYear: time.Now().Year(),
	}

	var htmlBodyTpl bytes.Buffer
	if err := tpl.Execute(&htmlBodyTpl, data); err != nil {
		log.Printf("Failed to execute template: %v", err)
		return err
	}

	htmlBodyTpl.WriteTo(w)

	return nil
}

func serveTemplate(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		mode := r.URL.Query().Get("mode")
		if mode == "raw" {
			_, _ = w.Write([]byte(templates[name]))
			return
		}

		if err := serveExecute(templates[name], w); err != nil {
			_, _ = w.Write([]byte(templates[name]))
		}
	}
}

func main() {
	http.HandleFunc("/", homepage)

	for _, tpl := range email.Templates() {
		templates[tpl.Name()] = tpl.ContentHTML()
	}

	for k := range templates {
		http.HandleFunc("/"+k, serveTemplate(k))
	}

	log.Println("Listening at http://localhost:8082/")

	_ = http.ListenAndServe(":8082", nil)
}
