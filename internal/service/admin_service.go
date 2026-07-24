package service

import (
	"XFeedSystem/internal/repo"
	"context"
	"errors"
)

type AdminService struct {
	repo *repo.GormAdminRepo
}

func NewAdminService(r *repo.GormAdminRepo) *AdminService {
	return &AdminService{repo: r}
}

type UserListItem struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Role      int8   `json:"role"`
	Status    int8   `json:"status"`
	CreatedAt string `json:"created_at"`
}

type UserListResponse struct {
	List       []*UserListItem `json:"list"`
	NextCursor int64           `json:"next_cursor"`
	Total      int64           `json:"total"`
}

func (s *AdminService) ListUsers(ctx context.Context, cursor int64, limit int, keyword string) (*UserListResponse, error) {
	users, err := s.repo.ListUsers(ctx, cursor, limit, keyword)
	if err != nil {
		return nil, err
	}
	total, _ := s.repo.CountUsers(ctx, keyword)
	list := make([]*UserListItem, 0, len(users))
	var nextCursor int64
	for _, u := range users {
		list = append(list, &UserListItem{
			ID:        u.ID,
			Username:  u.Username,
			AvatarURL: u.AvatarURL,
			Role:      u.Role,
			Status:    u.Status,
			CreatedAt: u.CreatedAt.Format("2006-01-02 15:04:05"),
		})
		nextCursor = u.ID
	}
	return &UserListResponse{List: list, NextCursor: nextCursor, Total: total}, nil
}

func (s *AdminService) BanUser(ctx context.Context, operatorRole int8, id int64) error {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if user.Role >= operatorRole {
		return errors.New("不能操作更高级管理员")
	}
	newStatus := int8(0)
	if user.Status == 0 {
		newStatus = 1
	}
	return s.repo.UpdateUserStatus(ctx, id, newStatus)
}

func (s *AdminService) DeleteUser(ctx context.Context, id int64, operatorRole int8) error {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if user.Role >= operatorRole {
		return errors.New("不能操作更高级管理员")
	}
	return s.repo.DeleteUser(ctx, id)
}

func (s *AdminService) DeleteNote(ctx context.Context, id int64) error {
	return s.repo.DeleteNoteByID(ctx, id)
}
func (s *AdminService) DeleteComment(ctx context.Context, id int64) error {
	return s.repo.DeleteCommentByID(ctx, id)
}

type SystemStats struct {
	UserCount    int64 `json:"user_count"`
	NoteCount    int64 `json:"note_count"`
	CommentCount int64 `json:"comment_count"`
}

func (s *AdminService) Stats(ctx context.Context) (*SystemStats, error) {
	users, _ := s.repo.CountUsers(ctx, "")
	notes, _ := s.repo.CountNotes(ctx)
	comments, _ := s.repo.CountComments(ctx)
	return &SystemStats{
		UserCount:    users,
		NoteCount:    notes,
		CommentCount: comments,
	}, nil
}
