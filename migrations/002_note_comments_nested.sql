-- 002_note_comments_nested.sql
-- 嵌套评论支持（兼容已有数据库，缺失列时补充）

SET @stmt = (SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE note_comments ADD COLUMN parent_id BIGINT NOT NULL DEFAULT 0',
    'SELECT 1'
) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'note_comments' AND COLUMN_NAME = 'parent_id');
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @stmt = (SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE note_comments ADD COLUMN reply_to_user_id BIGINT NOT NULL DEFAULT 0',
    'SELECT 1'
) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'note_comments' AND COLUMN_NAME = 'reply_to_user_id');
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 索引（使用 EXISTS 判断避免重复创建）
SET @stmt = (SELECT IF(
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
     WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'note_comments' AND INDEX_NAME = 'idx_note_comments_parent_id') = 0,
    'CREATE INDEX idx_note_comments_parent_id ON note_comments(parent_id)',
    'SELECT 1'
));
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @stmt = (SELECT IF(
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
     WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'note_comments' AND INDEX_NAME = 'idx_note_comments_reply_to_user_id') = 0,
    'CREATE INDEX idx_note_comments_reply_to_user_id ON note_comments(reply_to_user_id)',
    'SELECT 1'
));
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;
