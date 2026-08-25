CREATE TABLE IF NOT EXISTS feed_hides (
id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
user_id    BIGINT UNSIGNED NOT NULL,
note_id    BIGINT UNSIGNED NOT NULL,
type       TINYINT NOT NULL,               -- 冗余存笔记类型，算类型权重用
created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
UNIQUE KEY uk_user_note (user_id, note_id),  -- 一人一笔记只记一次（幂等）
KEY idx_user_type (user_id, type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;