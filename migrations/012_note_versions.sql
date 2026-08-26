-- migrations/012_note_versions.sql
CREATE TABLE IF NOT EXISTS note_versions (
id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
note_id        BIGINT UNSIGNED NOT NULL,
author_id      BIGINT UNSIGNED NOT NULL,
title          VARCHAR(255) NOT NULL DEFAULT '',
content        TEXT NOT NULL,
images         JSON NULL,
video_url      TEXT NULL,
type           TINYINT NOT NULL DEFAULT 1,
content_format TINYINT NOT NULL DEFAULT 1,
created_at     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
KEY idx_note (note_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;