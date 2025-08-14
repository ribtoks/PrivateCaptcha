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
}

type ScheduledNotifications interface {
	Add(ctx context.Context, notification *ScheduledNotification)
	Remove(ctx context.Context, userID int32, referenceID string)
}
