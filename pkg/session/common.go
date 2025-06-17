package session

import "github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"

const (
	KeyLoginStep common.SessionKey = iota
	KeyUserID
	KeyUserEmail
	KeyTwoFactorCode
	KeyUserName
	KeyPersistent
	KeyNotificationID
	KeyReturnURL
	// Add new fields _above_
	SESSION_KEYS_COUNT
)
