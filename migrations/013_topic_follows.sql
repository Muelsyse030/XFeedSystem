CREATE TABLE IF NOT EXISTS topic_follows (
id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
user_id    BIGINT UNSIGNED NOT NULL,
topic_id   BIGINT UNSIGNED NOT NULL,
created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
UNIQUE KEY uk_user_topic (user_id, topic_id),  -- 幂等：重复关注不报错
KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;