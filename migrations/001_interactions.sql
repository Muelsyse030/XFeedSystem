-- 001_interactions_mysql.sql

CREATE TABLE IF NOT EXISTS note_likes (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  note_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (note_id, user_id)
);

CREATE INDEX idx_note_likes_note_id ON note_likes(note_id);
CREATE INDEX idx_note_likes_user_id ON note_likes(user_id);

CREATE TABLE IF NOT EXISTS note_favorites (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  note_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (note_id, user_id)
);

CREATE INDEX idx_note_favorites_user_id_id ON note_favorites(user_id, id DESC);
CREATE INDEX idx_note_favorites_note_id ON note_favorites(note_id);

CREATE TABLE IF NOT EXISTS note_comments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  note_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  content TEXT NOT NULL,
  status SMALLINT NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_note_comments_note_id_id ON note_comments(note_id, id DESC);
CREATE INDEX idx_note_comments_user_id ON note_comments(user_id);

ALTER TABLE notes
  ADD COLUMN like_count BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN favorite_count BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN comment_count BIGINT NOT NULL DEFAULT 0;