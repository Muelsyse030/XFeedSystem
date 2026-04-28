-- 003_note_counters.sql
-- Add counter columns to notes table.
-- Compatible with MySQL versions that do not support IF NOT EXISTS for ADD COLUMN.

ALTER TABLE notes
  ADD COLUMN like_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE notes
  ADD COLUMN favorite_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE notes
  ADD COLUMN comment_count BIGINT NOT NULL DEFAULT 0;
