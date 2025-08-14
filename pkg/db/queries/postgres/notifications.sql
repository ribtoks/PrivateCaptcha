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
INSERT INTO backend.notification_templates (name, content, content_hash)
VALUES ($1, $2, $3)
ON CONFLICT (content_hash) DO UPDATE SET updated_at = NOW()
RETURNING *;

-- name: GetNotificationTemplateByHash :one
SELECT * FROM backend.notification_templates WHERE content_hash = $1;

-- name: CreateUserNotification :one
INSERT INTO backend.user_notifications (user_id, reference_id, template_hash, subject, payload, scheduled_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateSentUserNotifications :exec
UPDATE backend.user_notifications SET delivered_at = $1 WHERE id = ANY($2::INT[]);

-- name: GetPendingUserNotifications :many
SELECT sqlc.embed(un), u.email
FROM backend.user_notifications un
JOIN backend.users u ON un.user_id = u.id
WHERE delivered_at IS NULL AND scheduled_at >= $1 AND scheduled_at <= NOW() ORDER BY scheduled_at ASC
LIMIT $2;

-- name: DeleteUnusedNotificationTemplates :exec
DELETE FROM backend.notification_templates nt
WHERE nt.id IN (
    SELECT nt2.id
    FROM backend.notification_templates nt2
    LEFT JOIN backend.user_notifications un ON un.template_hash = nt2.content_hash
    WHERE un.template_hash IS NULL
    AND nt2.updated_at < $1
);

-- name: DeleteSentUserNotifications :exec
DELETE FROM backend.user_notifications
WHERE delivered_at IS NOT NULL
AND delivered_at < $1;

-- name: DeleteUnsentUserNotifications :exec
DELETE FROM backend.user_notifications
WHERE delivered_at IS NULL
AND scheduled_at < $1;
