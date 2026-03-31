package model

import "time"

type FeedItem struct {
	ID          int64      `json:"id"`
	AuthorID    int64      `json:"author_id"`
	Author      AuthorInfo `json:"author"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Type        int8       `json:"type"`
	PublishedAt time.Time  `json:"published_at"`
}

type FeedCursor struct {
	PublishedAt time.Time
	ID          int64
}

type AuthorInfo struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}
