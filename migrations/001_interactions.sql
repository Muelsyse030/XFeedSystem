-- 001_interactions.sql
-- 互动相关表：点赞、收藏、评论
-- 同时补充 notes 表的计数器列（兼容已有数据库）

CREATE TABLE IF NOT EXISTS note_likes (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id    BIGINT   NOT NULL,
    user_id    BIGINT   NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_note_user_like (note_id, user_id),
    INDEX idx_note_likes_note_id (note_id),
    INDEX idx_note_likes_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS note_favorites (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id    BIGINT   NOT NULL,
    user_id    BIGINT   NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_note_user_favorite (note_id, user_id),
    INDEX idx_note_favorites_note_id (note_id),
    INDEX idx_note_favorites_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS note_comments (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    note_id         BIGINT   NOT NULL,
    user_id         BIGINT   NOT NULL,
    parent_id       BIGINT   NOT NULL DEFAULT 0,
    reply_to_user_id BIGINT  NOT NULL DEFAULT 0,
    content         TEXT     NOT NULL,
    status          SMALLINT NOT NULL DEFAULT 1,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_note_comments_note_id_id (note_id, parent_id, id),
    INDEX idx_note_comments_user_id (user_id),
    INDEX idx_note_comments_parent_id (parent_id),
    INDEX idx_note_comments_reply_to_user_id (reply_to_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 计数器列（兼容已存在的 notes 表但缺少计数器的场景）
SET @stmt = (SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE notes ADD COLUMN like_count BIGINT NOT NULL DEFAULT 0',
    'SELECT 1'
) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notes' AND COLUMN_NAME = 'like_count');
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @stmt = (SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE notes ADD COLUMN favorite_count BIGINT NOT NULL DEFAULT 0',
    'SELECT 1'
) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notes' AND COLUMN_NAME = 'favorite_count');
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @stmt = (SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE notes ADD COLUMN comment_count BIGINT NOT NULL DEFAULT 0',
    'SELECT 1'
) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notes' AND COLUMN_NAME = 'comment_count');
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;
