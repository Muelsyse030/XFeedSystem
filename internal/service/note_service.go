package service

import (
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"errors"
	"gorm.io/gorm"
	"strings"
	"time"
)

var (
	ErrInvalidUserID    = errors.New("invalid user id")
	ErrInvalidNoteID    = errors.New("invalid note id")
	ErrInvalidCommentID = errors.New("invalid comment id")
	ErrInvalidComment   = errors.New("invalid comment content")
	ErrNoteNotFound     = errors.New("note not found")
	ErrCommentNotFound  = errors.New("comment not found")
)

type NoteService struct {
	repo repo.NoteRepo
}

func NewNoteService(r repo.NoteRepo) *NoteService {
	return &NoteService{repo: r}
}

func (s *NoteService) Create(userID int64, title, content string) (*model.Note, error) {
	note := &model.Note{
		AuthorID:    userID,
		Title:       title,
		Content:     content,
		Type:        1, //1默认为文章
		PublishedAt: time.Now(),
	}
	if _, err := s.repo.Create(note); err != nil {
		return nil, err
	}
	return note, nil
}

func (s *NoteService) ListByAuthorID(ctx context.Context, authorID, cursor int64, limit int) ([]*model.Note, error) {
	return s.repo.ListByAuthorID(ctx, authorID, cursor, limit)
}

func (s *NoteService) GetByID(ctx context.Context, id int64) (*model.Note, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *NoteService) Delete(ctx context.Context, id int64, authorID int64) error {
	return s.repo.DeleteByID(ctx, id, authorID)
}

func (s *NoteService) Like(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	if _, err := s.repo.GetByID(ctx, noteID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	_, err := s.repo.Like(ctx, noteID, userID)

	return true, err
}

func (s *NoteService) Unlike(ctx context.Context, noteID, userID int64) error {
	if userID <= 0 {
		return ErrInvalidUserID
	}
	if _, err := s.repo.GetByID(ctx, noteID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNoteNotFound
		}
		return err
	}
	_, err := s.repo.Unlike(ctx, noteID, userID)
	return err
}

func (s *NoteService) Favorite(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	if _, err := s.repo.GetByID(ctx, noteID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	_, err := s.repo.Favorite(ctx, noteID, userID)
	return true, err
}

func (s *NoteService) Unfavorite(ctx context.Context, noteID, userID int64) error {
	if userID <= 0 {
		return ErrInvalidUserID
	}
	if _, err := s.repo.GetByID(ctx, noteID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNoteNotFound
		}
		return err
	}
	_, err := s.repo.Unfavorite(ctx, noteID, userID)
	return err
}

func (s *NoteService) ListFavorites(ctx context.Context, userID, cursor int64, limit int) ([]*model.Note, int64, error) {
	if userID <= 0 {
		return nil, 0, ErrInvalidUserID
	}
	return s.repo.FavoriteList(ctx, userID, cursor, limit)
}
func (s *NoteService) CreateComment(ctx context.Context, userID, noteID int64, content string) (*model.NoteComment, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if strings.TrimSpace(content) == "" {
		return nil, ErrInvalidComment
	}
	if _, err := s.repo.GetByID(ctx, noteID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}
	return s.repo.CreateComment(ctx, userID, noteID, content)
}
func (s *NoteService) ListCommentsByNoteID(ctx context.Context, noteID, cursor int64, limit int) ([]*model.NoteComment, error) {
	if noteID <= 0 {
		return nil, ErrInvalidNoteID
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return s.repo.ListCommentsByNoteID(ctx, noteID, cursor, limit)
}
func (s *NoteService) DeleteComment(ctx context.Context, commentID int64, userID int64) error {
	if commentID <= 0 {
		return ErrInvalidCommentID
	}
	if _, err := s.repo.GetCommentByID(ctx, commentID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	if err := s.repo.DeleteComment(ctx, commentID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	return nil
}
