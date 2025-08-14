CREATE TABLE IF NOT EXISTS backend.user_notifications(
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES backend.users(id) ON DELETE CASCADE,
    template_hash VARCHAR(64) REFERENCES backend.notification_templates(content_hash),
    payload JSONB NOT NULL,
    subject TEXT NOT NULL,
    reference_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    scheduled_at TIMESTAMPTZ NOT NULL,
    delivered_at TIMESTAMPTZ DEFAULT NULL
);

-- Prevents duplicate undelivered notifications for the same user/reference_id, BUT allows multiple delivered notifications
CREATE UNIQUE INDEX index_unique_reference_per_user ON backend.user_notifications (user_id, reference_id) WHERE delivered_at IS NULL;
