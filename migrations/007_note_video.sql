-- 007_note_video.sql
-- notes 表新增 video_url 列（视频笔记），兼容已有数据库
SET @col_exists := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notes' AND COLUMN_NAME = 'video_url'
);
SET @ddl := IF(@col_exists = 0,
  'ALTER TABLE notes ADD COLUMN video_url TEXT NULL COMMENT ''视频地址(OSS)''',
  'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
