-- 000_baseline.sql
-- 基础建表：users, notes, follows, notifications
-- 后续增量迁移（001-006）在此基础上进行

CREATE TABLE IF NOT EXISTS users (
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    username      VARCHAR(64)  NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    avatar_url    VARCHAR(255) DEFAULT '',
    bio           VARCHAR(255) DEFAULT '',
    role          TINYINT      NOT NULL DEFAULT 0  COMMENT '0=普通用户, 1=管理员, 2=超级管理员',
    status        TINYINT      NOT NULL DEFAULT 1  COMMENT '0=封禁, 1=正常',
    created_at    DATETIME     DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX uk_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS notes (
    id             BIGINT PRIMARY KEY AUTO_INCREMENT,
    author_id      BIGINT       NOT NULL,
    title          VARCHAR(255) NOT NULL DEFAULT '',
    content        TEXT         NOT NULL,
    images         JSON         NULL,
    status         TINYINT      NOT NULL DEFAULT 1  COMMENT '1=已发布, 2=已删除',
    type           TINYINT      NOT NULL DEFAULT 1,
    like_count     BIGINT       NOT NULL DEFAULT 0,
    favorite_count BIGINT       NOT NULL DEFAULT 0,
    comment_count  BIGINT       NOT NULL DEFAULT 0,
    created_at     DATETIME     DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME     DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    published_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_notes_author_id (author_id),
    INDEX idx_notes_published_at (published_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS follows (
    user_id    BIGINT   NOT NULL,
    follow_id  BIGINT   NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, follow_id),
    INDEX idx_follows_user_id (user_id),
    INDEX idx_follows_follow_id (follow_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS notifications (
    id             BIGINT       PRIMARY KEY AUTO_INCREMENT,
    user_id        BIGINT       NOT NULL,
    actor_id       BIGINT       NOT NULL,
    type           TINYINT      NOT NULL COMMENT '1=点赞, 2=评论, 3=回复, 4=关注, 5=收藏',
    target_id      BIGINT       NOT NULL,
    target_note_id BIGINT       NOT NULL DEFAULT 0,
    message        VARCHAR(255) NOT NULL,
    is_read        TINYINT(1)   NOT NULL DEFAULT 0,
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_notif_user_read (user_id, is_read),
    INDEX idx_notif_actor_id (actor_id),
    INDEX idx_notif_type (type),
    INDEX idx_notif_target_id (target_id),
    INDEX idx_notif_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
