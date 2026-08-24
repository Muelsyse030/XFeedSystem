-- migrations/008_note_content_format.sql
-- notes 表新增 content_format 列（1=纯文本 2=富文本HTML），兼容已有数据库
SET @stmt = (SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE notes ADD COLUMN content_format TINYINT NOT NULL DEFAULT 1 COMMENT ''1=纯文本 2=富文本''',
    'SELECT 1'
) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notes' AND COLUMN_NAME = 'content_format');
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;