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

type NotificationCondition int

const (
	EmptyNotificationCondition NotificationCondition = iota
	NotificationWithSubscription
	NotificationWithoutSubscription
)

type ScheduledNotification struct {
	ReferenceID  string
	UserID       int32
	Subject      string
	Data         interface{}
	DateTime     time.Time
	TemplateHash string
	Persistent   bool
	Condition    NotificationCondition
}

func NewEmailTemplate(name, contentHTML, contentText string) *EmailTemplate {
	return &EmailTemplate{
		name:        name,
		contentHTML: contentHTML,
		contentText: contentText,
	}
}

type EmailTemplate struct {
	name        string
	hash        string
	mux         sync.Mutex
	contentHTML string
	contentText string
}

func (et *EmailTemplate) Name() string        { return et.name }
func (et *EmailTemplate) ContentHTML() string { return et.contentHTML }
func (et *EmailTemplate) ContentText() string { return et.contentText }

func (et *EmailTemplate) Hash() string {
	et.mux.Lock()
	defer et.mux.Unlock()

	if len(et.hash) == 0 {
		h := sha1.New()
		if len(et.contentHTML) > 0 {
			h.Write([]byte(et.contentHTML))
		} else if len(et.contentText) > 0 {
			h.Write([]byte(et.contentText))
		} else {
			h.Write([]byte(et.name))
		}
		et.hash = hex.EncodeToString(h.Sum(nil))
	}

	return et.hash
}
