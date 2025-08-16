-- name: GetSystemNotificationById :one
SELECT * FROM backend.system_notifications WHERE id = $1;

-- name: GetLastActiveSystemNotification :one
SELECT * FROM backend.system_notifications
 WHERE is_active = TRUE AND
   start_date <= $1::timestamptz AND
   (end_date IS NULL OR end_date > $1::timestamptz) AND
   (user_id = $2 OR user_id IS NULL)
 ORDER BY
   CASE WHEN user_id = $2 THEN 0 ELSE 1 END,
   start_date DESC
 LIMIT 1;

-- name: CreateSystemNotification :one
INSERT INTO backend.system_notifications (message, start_date, end_date, user_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateNotificationTemplate :one
INSERT INTO backend.notification_templates (name, content_html, content_text, external_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (external_id) DO UPDATE SET updated_at = NOW()
RETURNING *;

-- name: GetNotificationTemplateByHash :one
SELECT * FROM backend.notification_templates WHERE external_id = $1;

-- name: CreateUserNotification :one
INSERT INTO backend.user_notifications (user_id, reference_id, template_id, subject, payload, scheduled_at, persistent, requires_subscription)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: DeletePendingUserNotification :exec
DELETE FROM backend.user_notifications WHERE delivered_at IS NULL AND user_id = $1 AND reference_id = $2;

-- name: UpdateSentUserNotifications :exec
UPDATE backend.user_notifications SET delivered_at = $1 WHERE id = ANY($2::INT[]);

-- name: GetPendingUserNotifications :many
SELECT sqlc.embed(un), u.email
FROM backend.user_notifications un
JOIN backend.users u ON un.user_id = u.id
WHERE delivered_at IS NULL
  AND scheduled_at >= $1
  AND scheduled_at <= NOW()
  AND u.deleted_at IS NULL
  AND (un.requires_subscription IS NULL OR u.subscription_id IS NOT NULL)
ORDER BY scheduled_at ASC
LIMIT $2;

-- name: DeleteUnusedNotificationTemplates :exec
DELETE FROM backend.notification_templates nt
WHERE nt.id IN (
    SELECT nt2.id
    FROM backend.notification_templates nt2
    LEFT JOIN backend.user_notifications un ON un.template_id = nt2.external_id
    WHERE ((un.template_id IS NULL) OR (un.delivered_at < $1))
    AND (nt2.updated_at < $2)
);

-- name: DeleteSentUserNotifications :exec
DELETE FROM backend.user_notifications
WHERE delivered_at IS NOT NULL
AND persistent = false
AND delivered_at < $1;

-- name: DeleteUnsentUserNotifications :exec
DELETE FROM backend.user_notifications
WHERE delivered_at IS NULL
AND persistent = false
AND scheduled_at < $1;
