// Package events 定义跨进程的事件契约。
// API 进程只负责把业务事件写进 outbox 表；Redis Streams 上的三个消费组
// （feed/search/notify）各自读全量消息，再按 type 过滤出自己关心的部分。

package events

const (
	// StreamKey 是唯一的事件流。三个消费组各自独立推进游标，
	// 互不干扰，feed worker 落后不会拖累 notify worker。
	StreamKey = "xfeed:events"

	// GroupFeed 消费组
	GroupFeed   = "xfeed:feed"
	GroupSearch = "xfeed:search"
	GroupNotify = "xfeed:notify"

	// NoteCreated 事件类型
	NoteCreated     = "note.created"
	NoteUpdated     = "note.updated"
	NoteDeleted     = "note.deleted"
	NoteLiked       = "note.liked"
	NoteUnliked     = "note.unliked"
	NoteFavorited   = "note.favorited"
	NoteUnfavorited = "note.unfavorited"
	CommentCreated  = "comment.created"
	UserFollowed    = "user.followed"
	CommentDeleted  = "comment.deleted"
	UserUnfollowed  = "user.unfollowed"
)

// Payload 是所有事件共用的负载，字段按事件类型取用。
// 为什么不给每种事件各建一个 struct：消费端按 type 分发后需要的字段高度重叠，
// 共用一个 Payload 让 outbox 表、relay、worker 的序列化逻辑保持单一。
// 事件内容保持最小化：feed/search worker 需要的数据一律回 MySQL 查
// （MySQL 是真相源，事件里不搬正文等大字段）。
type Payload struct {
	Version int `json:"version"` // 契约版本，将来字段不兼容时用于平滑迁移

	EventID int64 `json:"event_id"` // outbox 行 ID，Notify Worker 用它做幂等去重

	NoteID   int64 `json:"note_id,omitempty"`
	AuthorID int64 `json:"author_id,omitempty"` // 笔记作者 / 被关注者（通知接收方）
	ActorID  int64 `json:"actor_id,omitempty"`  // 触发操作的用户

	CommentID     int64    `json:"comment_id,omitempty"`
	ParentID      int64    `json:"parent_id,omitempty"`
	ReplyToUserID int64    `json:"reply_to_user_id,omitempty"`
	MentionNames  []string `json:"mention_names,omitempty"` // @提及的用户名（纯正则提取，不查库）
}
