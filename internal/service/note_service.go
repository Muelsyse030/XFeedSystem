package service

import (
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"time"
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
