CREATE TABLE IF NOT EXISTS messages (
id                 BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
sender_id          BIGINT UNSIGNED NOT NULL,            -- 0=系统/管理员
receiver_id        BIGINT UNSIGNED NOT NULL,
content            TEXT NOT NULL,
client_message_id  VARCHAR(64) NOT NULL,                -- 前端生成的幂等键
is_read            TINYINT NOT NULL DEFAULT 0,
read_at            DATETIME(3) NULL,
sender_deleted     TINYINT NOT NULL DEFAULT 0,          -- 发件人侧软删
receiver_deleted   TINYINT NOT NULL DEFAULT 0,          -- 收件人侧软删
created_at         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
UNIQUE KEY uk_client_msg (sender_id, client_message_id),  -- 幂等兜底
KEY idx_receiver_created (receiver_id, id),             -- 收件箱游标翻页
KEY idx_sender_created  (sender_id, id),                -- 发件箱
KEY idx_peer            (sender_id, receiver_id, id)    -- 会话内翻页
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;