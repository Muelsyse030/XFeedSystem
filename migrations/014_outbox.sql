CREATE TABLE IF NOT EXISTS outbox_events (
id           BIGINT PRIMARY KEY AUTO_INCREMENT,
event_type   VARCHAR(64)  NOT NULL COMMENT '事件类型，如 note.created',
payload      JSON         NOT NULL COMMENT '事件负载（events.Payload 序列化）',
status       TINYINT      NOT NULL DEFAULT 0 COMMENT '0=待发布, 1=已发布',
attempts     INT          NOT NULL DEFAULT 0,
created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
published_at DATETIME     NULL,
KEY idx_outbox_status_id (status, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;