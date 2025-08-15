package common

import (
	"context"
	"time"
)

type Mailer interface {
	SendTwoFactor(ctx context.Context, email string, code int) error
	SendWelcome(ctx context.Context, email string) error
}

type ScheduledNotification struct {
	ReferenceID  string
	UserID       int32
	Subject      string
	Data         interface{}
	DateTime     time.Time
	TemplateName string
	Persistent   bool
}

type ScheduledNotifications interface {
	Add(ctx context.Context, notification *ScheduledNotification) error
	Remove(ctx context.Context, userID int32, referenceID string) error
}
