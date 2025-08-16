CREATE TABLE IF NOT EXISTS backend.user_notifications(
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES backend.users(id) ON DELETE CASCADE,
    template_id VARCHAR(64) REFERENCES backend.notification_templates(external_id) ON DELETE SET NULL,
    payload JSONB NOT NULL,
    subject TEXT NOT NULL,
    reference_id TEXT NOT NULL,
    persistent BOOL NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    scheduled_at TIMESTAMPTZ NOT NULL,
    delivered_at TIMESTAMPTZ DEFAULT NULL
);

-- Prevents duplicate undelivered notifications for the same user/reference_id IF it is
-- permanent notification or temporary notification that hasn't been delivered yet
CREATE UNIQUE INDEX index_unique_reference_per_user
ON backend.user_notifications (user_id, reference_id)
WHERE (persistent = true) OR (delivered_at IS NULL);

CREATE OR REPLACE TRIGGER deleted_record_insert AFTER DELETE ON backend.user_notifications
   FOR EACH ROW EXECUTE FUNCTION backend.deleted_record_insert();
