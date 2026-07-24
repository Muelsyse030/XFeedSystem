-- 004_note_images.sql
-- notes 表新增 images JSON 列

SET @stmt = (SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE notes ADD COLUMN images JSON NULL',
    'SELECT 1'
) FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notes' AND COLUMN_NAME = 'images');
PREPARE stmt FROM @stmt; EXECUTE stmt; DEALLOCATE PREPARE stmt;
