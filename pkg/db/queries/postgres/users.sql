-- name: GetUserByID :one
SELECT * FROM backend.users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM backend.users WHERE email = $1 AND deleted_at IS NULL;

-- name: GetUserBySubscriptionID :one
SELECT * FROM backend.users WHERE subscription_id = $1;

-- name: CreateUser :one
INSERT INTO backend.users (name, email, subscription_id) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateUserData :one
UPDATE backend.users SET name = $2, email = $3, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: UpdateUserSubscription :one
UPDATE backend.users SET subscription_id = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: SoftDeleteUser :one
UPDATE backend.users SET deleted_at = NOW() WHERE id = $1 RETURNING *;

-- name: GetSoftDeletedUsers :many
SELECT sqlc.embed(u)
FROM backend.users u
WHERE u.deleted_at IS NOT NULL
  AND u.deleted_at < $1
LIMIT $2;

-- name: DeleteUsers :exec
DELETE FROM backend.users WHERE id = ANY($1::INT[]);

-- name: GetUsersWithoutSubscription :many
SELECT * FROM backend.users where id = ANY($1::INT[]) AND (subscription_id IS NULL OR deleted_at IS NOT NULL);

-- name: GetUsersWithExpiredTrials :many
SELECT u.*
FROM backend.users u
JOIN backend.subscriptions s ON u.subscription_id = s.id
WHERE
  s.source = 'internal' AND
  s.trial_ends_at IS NOT NULL AND
  s.trial_ends_at BETWEEN $1 AND $2 AND
  s.status = $3 AND
  (s.external_customer_id IS NULL OR s.external_customer_id = '') AND
  (s.external_subscription_id IS NULL OR s.external_subscription_id = '') AND
  s.next_billed_at IS NULL AND
  u.deleted_at IS NULL
LIMIT $4;
