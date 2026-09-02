-- 通知幂等从 Redis SET NX 换成数据库唯一约束
ALTER TABLE notifications
    ADD COLUMN event_id BIGINT NULL DEFAULT NULL AFTER message,
    ADD UNIQUE INDEX uk_notif_event_user (event_id, user_id);