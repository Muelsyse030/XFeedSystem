CREATE TABLE IF NOT EXISTS reports (
id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
reporter_id      BIGINT UNSIGNED NOT NULL,           -- 举报人
target_type      TINYINT NOT NULL,                   -- 1=笔记 2=评论 3=用户 4=站内信
target_id        BIGINT UNSIGNED NOT NULL,
reason           TINYINT NOT NULL,                   -- 1=垃圾广告 2=色情低俗 3=人身攻击 4=违法违规 5=虚假信息 6=其他
description      VARCHAR(500) NOT NULL DEFAULT '',   -- 补充说明
target_snapshot  TEXT NULL,                          -- 被举报内容快照
status           TINYINT NOT NULL DEFAULT 0,         -- 0=待处理 1=已成立 2=已驳回
handled_by       BIGINT UNSIGNED NULL,
handled_at       DATETIME(3) NULL,
created_at       DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
UNIQUE KEY uk_reporter_target (reporter_id, target_type, target_id), -- 一人一目标一条
KEY idx_status_created (status, id),                 -- 管理员队列
KEY idx_target (target_type, target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;