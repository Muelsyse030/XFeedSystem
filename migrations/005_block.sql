-- 005_block.sql
-- 拉黑（屏蔽）关系表

CREATE TABLE IF NOT EXISTS blocks (
    user_id    BIGINT   NOT NULL,
    blocked_id BIGINT   NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, blocked_id),
    INDEX idx_blocks_user_id (user_id),
    INDEX idx_blocks_blocked_id (blocked_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
