package portal

import (
	"context"
	"testing"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/config"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	db_tests "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/tests"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/maintenance"
)

func TestUserNotificationsJob(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	t.Parallel()

	ctx := common.TraceContext(context.TODO(), t.Name())

	user, _, err := db_tests.CreateNewAccountForTest(ctx, store, t.Name(), testPlan)
	if err != nil {
		t.Fatalf("failed to create new account: %v", err)
	}

	tnow := time.Now().UTC()

	const referenceID = "referenceID"

	hash := db.EmailTemplateHash(email.TwoFactorHTMLTemplate)
	if _, err := store.Impl().CreateUserNotification(ctx, user.ID, referenceID, map[string]int{}, "subject", hash, tnow.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	sender := &email.StubSender{}

	job := &maintenance.UserEmailNotificationsJob{
		RunInterval:  1 * time.Hour,
		Store:        store,
		Templates:    email.Templates(),
		Sender:       sender,
		ChunkSize:    config.NewStaticValue(common.NotificationsChunkSizeKey, "100"),
		EmailFrom:    config.NewStaticValue(common.EmailFromKey, "foo@bar.com"),
		ReplyToEmail: config.NewStaticValue(common.ReplyToEmailKey, "foo@bar.com"),
	}

	if err := job.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	if sender.Count != 1 {
		t.Errorf("Unexpected number of sent emails: %v", sender.Count)
	}

	// run again, but the notification should be processed by now
	if err := job.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	if sender.Count != 1 {
		t.Errorf("Unexpected number of sent emails: %v", sender.Count)
	}
}

func TestDeleteSentNotifications(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	t.Parallel()

	ctx := common.TraceContext(context.TODO(), t.Name())

	user, _, err := db_tests.CreateNewAccountForTest(ctx, store, t.Name(), testPlan)
	if err != nil {
		t.Fatalf("failed to create new account: %v", err)
	}

	tnow := time.Now().UTC()

	const referenceID = "referenceID"

	hash := db.EmailTemplateHash(email.TwoFactorHTMLTemplate)
	notif, err := store.Impl().CreateUserNotification(ctx, user.ID, referenceID, map[string]int{}, "subject", hash, tnow.Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Impl().CreateUserNotification(ctx, user.ID, referenceID, map[string]int{}, "subject", hash, tnow.Add(-10*time.Minute)); err == nil {
		t.Fatal("Shouldn't create a notification with the same referenceID")
	}

	if err := store.Impl().MarkUserNotificationsSent(ctx, []int32{notif.ID}, tnow.Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if err := store.Impl().DeleteSentUserNotifications(ctx, tnow.Add(-1*time.Minute)); err != nil {
		t.Fatal(err)
	}

	// should be able to create again (unlike before)
	if _, err := store.Impl().CreateUserNotification(ctx, user.ID, referenceID, map[string]int{}, "subject", hash, tnow.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
}
