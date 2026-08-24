package repo

import (
	"XFeedSystem/internal/model"
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrMessageNotFound = errors.New("message not found")

type MessageRepo interface {
	Create(ctx context.Context, msg *model.Message) error
	GetByClientMsgID(ctx context.Context, senderID int64, clientMsgID string) (*model.Message, error)
	GetByID(ctx context.Context, id int64) (*model.Message, error)
	ListWithPeer(ctx context.Context, meID, peerID, cursor int64, limit int) ([]*model.Message, error)
	ListConversations(ctx context.Context, userID, cursor int64, limit int) ([]*model.ConversationRow, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*model.Message, error)
	UnreadCount(ctx context.Context, userID int64) (int64, error)
	UnreadCountWithPeer(ctx context.Context, userID, peerID int64) (int64, error)
	MarkReadWithPeer(ctx context.Context, userID, peerID int64) (int64, error)
	SoftDelete(ctx context.Context, id, userID int64) (int64, error)
}

type GormMessageRepo struct{ db *gorm.DB }

func NewGormMessageRepo(db *gorm.DB) *GormMessageRepo { return &GormMessageRepo{db: db} }

func (r *GormMessageRepo) Create(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *GormMessageRepo) GetByClientMsgID(ctx context.Context, senderID int64, clientMsgID string) (*model.Message, error) {
	var msg model.Message
	err := r.db.WithContext(ctx).Where("sender_id = ? AND client_message_id = ?", senderID, clientMsgID).First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *GormMessageRepo) GetByID(ctx context.Context, id int64) (*model.Message, error) {
	var msg model.Message
	if err := r.db.WithContext(ctx).First(&msg, id).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *GormMessageRepo) ListWithPeer(ctx context.Context, meID, peerID, cursor int64, limit int) ([]*model.Message, error) {
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	var msgs []*model.Message
	err := r.db.WithContext(ctx).
		Where(`((sender_id = ? AND receiver_id = ? AND sender_deleted = 0)
		        OR (receiver_id = ? AND sender_id = ? AND receiver_deleted = 0))
		       AND id < ?`, meID, peerID, meID, peerID, cursor).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error
	return msgs, err
}

func (r *GormMessageRepo) ListConversations(ctx context.Context, userID, cursor int64, limit int) ([]*model.ConversationRow, error) {
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	var rows []*model.ConversationRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT peer_id,
		       MAX(msg_id) AS last_msg_id,
		       SUM(CASE WHEN direction = 'in' AND is_read = 0 THEN 1 ELSE 0 END) AS unread
		FROM (
			SELECT id AS msg_id, receiver_id AS peer_id, is_read, 'out' AS direction
			FROM messages
			WHERE sender_id = ? AND sender_deleted = 0
			UNION ALL
			SELECT id AS msg_id, sender_id AS peer_id, is_read, 'in' AS direction
			FROM messages
			WHERE receiver_id = ? AND receiver_deleted = 0
		) t
		GROUP BY peer_id
		HAVING last_msg_id < ?
		ORDER BY last_msg_id DESC
		LIMIT ?`, userID, userID, cursor, limit).
		Scan(&rows).Error
	return rows, err
}

func (r *GormMessageRepo) GetByIDs(ctx context.Context, ids []int64) ([]*model.Message, error) {
	if len(ids) == 0 {
		return []*model.Message{}, nil
	}
	var msgs []*model.Message
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&msgs).Error
	return msgs, err
}

func (r *GormMessageRepo) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Message{}).
		Where("receiver_id = ? AND is_read = 0 AND receiver_deleted = 0", userID).
		Count(&n).Error
	return n, err
}
func (r *GormMessageRepo) UnreadCountWithPeer(ctx context.Context, userID, peerID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Message{}).
		Where("receiver_id = ? AND sender_id = ? AND is_read = 0 AND receiver_deleted = 0", userID, peerID).
		Count(&n).Error
	return n, err
}
func (r *GormMessageRepo) MarkReadWithPeer(ctx context.Context, userID, peerID int64) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.Message{}).
		Where("receiver_id = ? AND sender_id = ? AND is_read = 0 AND receiver_deleted = 0", userID, peerID).
		Updates(map[string]interface{}{"is_read": 1, "read_at": gorm.Expr("NOW(3)")})
	return res.RowsAffected, res.Error
}
func (r *GormMessageRepo) SoftDelete(ctx context.Context, id, userID int64) (int64, error) {
	var col string
	if err := r.db.WithContext(ctx).Model(&model.Message{}).
		Select("CASE WHEN receiver_id = ? THEN 'receiver_deleted' ELSE 'sender_deleted' END",
			userID).
		Where("id = ? AND (sender_id = ? OR receiver_id = ?)", id, userID, userID).
		Scan(&col).Error; err != nil {
		return 0, err
	}
	if col == "" {
		return 0, ErrMessageNotFound
	}
	res := r.db.WithContext(ctx).Model(&model.Message{}).
		Where("id = ?", id).
		UpdateColumn(col, 1)
	return res.RowsAffected, res.Error
}
