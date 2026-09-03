ALTER TABLE notes
    ADD INDEX idx_notes_author_published (author_id, published_at DESC, id DESC);

ALTER TABLE follows
    ADD INDEX idx_follows_follow_id_user_id (follow_id, user_id);