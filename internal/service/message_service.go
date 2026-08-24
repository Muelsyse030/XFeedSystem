package service

import (
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const MaxMessageLen = 2000

var (
	ErrCannotSendToSelf = errors.New("不能给自己发站内信")
	ErrInvalidMessage   = errors.New("站内信内容不能为空且不超过 2000 字")
	ErrMessageNotFound  = errors.New("消息不存在")
	ErrReceiverNotFound = errors.New("接收用户不存在")
)

// isDuplicateKey 判断是否为唯一键冲突（幂等重发场景）。
// GORM 的 ErrDuplicatedKey 在部分版本/驱动下不生效，这里再兜底识别 MySQL 1062。
func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

type MessageService struct {
	repo     *repo.GormMessageRepo
	userRepo *repo.GormUserRepo
	block    *BlockService
}

func NewMessageService(r *repo.GormMessageRepo, u *repo.GormUserRepo, b *BlockService) *MessageService {
	return &MessageService{repo: r, userRepo: u, block: b}
}

func (s *MessageService) Send(ctx context.Context, senderID, receiverID int64, content, clientMsgID string) (*model.Message, error) {
	if senderID == receiverID {
		return nil, ErrCannotSendToSelf
	}
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > MaxMessageLen || strings.TrimSpace(clientMsgID) == "" {
		return nil, ErrInvalidMessage
	}
	if users, err := s.userRepo.GetByIDs([]int64{receiverID}); err != nil || len(users) == 0 {
		return nil, ErrReceiverNotFound
	}
	if s.block != nil {
		if blocked, err := s.block.IsBlockedEitherWay(ctx, senderID, receiverID); err == nil && blocked {
			return nil, ErrBlocked
		}
	}
	msg := &model.Message{
		SenderID:        senderID,
		ReceiverID:      receiverID,
		Content:         content,
		ClientMessageID: clientMsgID,
		IsRead:          0,
		CreatedAt:       time.Now(),
	}
	if err := s.repo.Create(ctx, msg); err != nil {
		if isDuplicateKey(err) {
			// 幂等：同键已存在，返回原消息而不是报错
			return s.repo.GetByClientMsgID(ctx, senderID, clientMsgID)
		}
		return nil, err
	}
	return msg, nil
}

func (s *MessageService) ListConversations(ctx context.Context, userID, cursor int64, limit int) ([]gin.H, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.repo.ListConversations(ctx, userID, cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	lastIDs := make([]int64, 0, len(rows))
	peerIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		lastIDs = append(lastIDs, r.LastMsgID)
		peerIDs = append(peerIDs, r.PeerID)
	}
	msgs, _ := s.repo.GetByIDs(ctx, lastIDs)
	byID := make(map[int64]*model.Message, len(msgs))
	for _, m := range msgs {
		byID[m.ID] = m
	}
	users, _ := s.userRepo.GetByIDs(peerIDs)
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	list := make([]gin.H, 0, len(rows))
	var nextCursor int64
	for _, r := range rows {
		m := byID[r.LastMsgID]
		u := userMap[r.PeerID]
		item := gin.H{
			"peer_id":      r.PeerID,
			"last_message": "",
			"last_at":      nil,
			"unread_count": r.Unread,
			"direction":    "out",
		}
		if m != nil {
			item["last_message"] = m.Content
			item["last_at"] = m.CreatedAt
			if m.SenderID != userID {
				item["direction"] = "in"
			}
		}
		if u != nil {
			item["peer"] = gin.H{
				"id":         u.ID,
				"username":   u.Username,
				"avatar_url": u.AvatarURL,
			}
		}
		list = append(list, item)
		nextCursor = r.LastMsgID
	}
	return list, nextCursor, nil
}

func (s *MessageService) ListWithPeer(ctx context.Context, meID, peerID, cursor int64, limit int) ([]gin.H, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	msgs, err := s.repo.ListWithPeer(ctx, meID, peerID, cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	list := make([]gin.H, 0, len(msgs))
	var nextCursor int64
	for _, m := range msgs {
		direction := "in"
		if m.SenderID == meID {
			direction = "out"
		}
		list = append(list, gin.H{
			"id":         m.ID,
			"direction":  direction,
			"content":    m.Content,
			"is_read":    m.IsRead,
			"created_at": m.CreatedAt,
		})
		nextCursor = m.ID
	}
	return list, nextCursor, nil
}

func (s *MessageService) MarkRead(ctx context.Context, meID, peerID int64) error {
	_, err := s.repo.MarkReadWithPeer(ctx, meID, peerID)
	return err
}

func (s *MessageService) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	return s.repo.UnreadCount(ctx, userID)
}

func (s *MessageService) Delete(ctx context.Context, id, userID int64) error {
	n, err := s.repo.SoftDelete(ctx, id, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMessageNotFound
	}
	return nil
}
