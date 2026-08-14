package repo

import (
	"XFeedSystem/internal/model"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StatsRepo interface {
	AddCounters(ctx context.Context, noteID int64, impressions, reads int64) error
	GetByNoteIDs(ctx context.Context, noteIDs []int64) (map[int64]*model.NoteStats, error)
}

type GormStatsRepo struct {
	db *gorm.DB
}

func NewGormStatsRepo(db *gorm.DB) *GormStatsRepo {
	return &GormStatsRepo{db: db}
}

func (r *GormStatsRepo) AddCounters(ctx context.Context, noteID int64, impressions, reads int64) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "note_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"impressions": gorm.Expr("impressions + ?", impressions),
			"reads":       gorm.Expr("reads + ?", reads),
		}),
	}).Create(&model.NoteStats{NoteID: noteID, Impressions: impressions, Reads: reads}).Error
}

func (r *GormStatsRepo) GetByNoteIDs(ctx context.Context, noteIDs []int64) (map[int64]*model.NoteStats, error) {
	out := make(map[int64]*model.NoteStats)
	if len(noteIDs) == 0 {
		return out, nil
	}
	var rows []model.NoteStats
	if err := r.db.WithContext(ctx).Where("note_id IN ?", noteIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].NoteID] = &rows[i]
	}
	return out, nil
}