package common

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"sync"
	"time"
)

type Mailer interface {
	SendTwoFactor(ctx context.Context, email string, code int) error
	SendWelcome(ctx context.Context, email, name string) error
}

type ScheduledNotification struct {
	ReferenceID  string
	UserID       int32
	Subject      string
	Data         interface{}
	DateTime     time.Time
	TemplateHash string
	Persistent   bool
}

func NewEmailTemplate(name, content string) *EmailTemplate {
	return &EmailTemplate{name: name, content: content}
}

type EmailTemplate struct {
	name    string
	hash    string
	mux     sync.Mutex
	content string
}

func (et *EmailTemplate) Name() string    { return et.name }
func (et *EmailTemplate) Content() string { return et.content }

func (et *EmailTemplate) Hash() string {
	et.mux.Lock()
	defer et.mux.Unlock()

	if len(et.hash) == 0 {
		h := sha1.New()
		h.Write([]byte(et.content))
		et.hash = hex.EncodeToString(h.Sum(nil))
	}

	return et.hash
}
