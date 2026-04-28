-- 002_note_comments_nested.sql
-- Incremental migration for nested comments / floor-in-floor support.
-- Compatible with MySQL versions that do not support IF NOT EXISTS for ADD COLUMN.

ALTER TABLE note_comments
  ADD COLUMN parent_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE note_comments
  ADD COLUMN reply_to_user_id BIGINT NOT NULL DEFAULT 0;

-- idx_note_comments_note_id_id already exists in 001_interactions.sql.
-- Keep the following index additions separate to avoid duplicate key errors.
CREATE INDEX idx_note_comments_parent_id
  ON note_comments(parent_id);
CREATE INDEX idx_note_comments_reply_to_user_id
  ON note_comments(reply_to_user_id);
