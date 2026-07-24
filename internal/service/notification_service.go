package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"log"
	"strconv"
	"time"
)

type NotificationService struct {
	repo     repo.NotificationRepo
	userRepo repo.UserRepo
	cache    *cache.RedisCache
}

func NewNotificationService(r repo.NotificationRepo, ur repo.UserRepo, c *cache.RedisCache) *NotificationService {
	return &NotificationService{repo: r, userRepo: ur, cache: c}
}

func (s *NotificationService) Create(ctx context.Context, actorID, userID int64, typ int8, targetID, targetNoteID int64, message string) {
	if actorID == userID {
		return
	}
	notif := &model.Notification{
		UserID:       userID,
		ActorID:      actorID,
		Type:         typ,
		TargetID:     targetID,
		TargetNoteID: targetNoteID,
		Message:      message,
	}
	if err := s.repo.Create(ctx, notif); err != nil {
		log.Printf("create notification failed :%v", err)
		return
	}
	if s.cache != nil {
		_, _ = s.cache.Incr(ctx, cache.NotifUnreadKey(userID))
	}
}

type NotifItem struct {
	ID           int64             `json:"id"`
	Type         int8              `json:"type"`
	Actor        *model.AuthorInfo `json:"actor"`
	TargetID     int64             `json:"target_id"`
	TargetNoteID int64             `json:"target_note_id"`
	Message      string            `json:"message"`
	IsRead       bool              `json:"is_read"`
	CreatedAt    time.Time         `json:"created_at"`
}

type NotifListResponse struct {
	List       []*NotifItem `json:"list"`
	NextCursor int64        `json:"next_cursor"`
}

func (s *NotificationService) List(ctx context.Context, userID int64, cursor int64, limit int) (*NotifListResponse, error) {
	notifs, err := s.repo.ListByUser(ctx, userID, cursor, limit)
	if err != nil {
		return nil, err
	}
	if len(notifs) == 0 {
		return &NotifListResponse{List: []*NotifItem{}, NextCursor: 0}, nil
	}

	actorIDs := make([]int64, len(notifs))
	for i, n := range notifs {
		actorIDs[i] = n.ActorID
	}
	users, _ := s.userRepo.GetByIDs(actorIDs)
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]*NotifItem, 0, len(notifs))
	for _, n := range notifs {
		item := &NotifItem{
			ID:           n.ID,
			Type:         n.Type,
			TargetID:     n.TargetID,
			TargetNoteID: n.TargetNoteID,
			Message:      n.Message,
			IsRead:       n.IsRead,
			CreatedAt:    n.CreatedAt,
		}
		if u, ok := userMap[n.ActorID]; ok {
			item.Actor = &model.AuthorInfo{
				ID:        u.ID,
				Username:  u.Username,
				AvatarURL: u.AvatarURL,
			}
		}
		items = append(items, item)
	}

	nextCursor := notifs[len(notifs)-1].ID
	return &NotifListResponse{List: items, NextCursor: nextCursor}, nil
}

func (s *NotificationService) MarkRead(ctx context.Context, id int64, userID int64) error {
	if err := s.repo.MarkRead(ctx, id, userID); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, cache.NotifUnreadKey(userID))
	}
	return nil
}

func (s *NotificationService) MarkAllRead(ctx context.Context, userID int64) error {
	if err := s.repo.MarkAllRead(ctx, userID); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, cache.NotifUnreadKey(userID))
	}
	return nil
}

func (s *NotificationService) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	if s.cache != nil {
		val, err := s.cache.Get(ctx, cache.NotifUnreadKey(userID))
		if err == nil {
			count, err2 := strconv.ParseInt(val, 10, 64)
			if err2 == nil {
				return count, nil
			}
		}
	}
	count, err := s.repo.CountUnread(ctx, userID)
	if err != nil {
		return 0, err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, cache.NotifUnreadKey(userID), strconv.FormatInt(count, 10), 5*time.Minute)
	}
	return count, nil
}
