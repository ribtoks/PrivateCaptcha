-- name: GetSubscriptionByID :one
SELECT * FROM backend.subscriptions WHERE id = $1;

-- name: GetSubscriptionsByUserIDs :many
SELECT sqlc.embed(s), u.id AS user_id
FROM backend.subscriptions s
JOIN backend.users u on u.subscription_id = s.id
WHERE u.id = ANY($1::INT[]) AND u.subscription_id IS NOT NULL;

-- name: CreateSubscription :one
INSERT INTO backend.subscriptions (external_product_id, external_price_id, external_subscription_id, external_customer_id, status, source, trial_ends_at, next_billed_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: UpdateSubscription :one
UPDATE backend.subscriptions SET external_product_id = $2, status = $3, next_billed_at = $4, cancel_from = $5, updated_at = NOW() WHERE external_subscription_id = $1 RETURNING *;

-- name: UpdateInternalSubscriptions :exec
UPDATE backend.subscriptions
SET status = $1, updated_at = NOW()
WHERE
  source = 'internal' AND
  trial_ends_at IS NOT NULL AND
  trial_ends_at BETWEEN $2 AND $3 AND
  status = $4 AND
  (external_customer_id IS NULL OR external_customer_id = '') AND
  (external_subscription_id IS NULL OR external_subscription_id = '') AND
  next_billed_at IS NULL;
