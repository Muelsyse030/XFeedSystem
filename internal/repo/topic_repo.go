package repo

import (
	"XFeedSystem/internal/model"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TopicRepo interface {
	UpsertByName(ctx context.Context, names []string) (map[string]int64, error)
	AttachNoteTopics(ctx context.Context, noteID int64, topicIDs []int64) error
	DetachNoteTopics(ctx context.Context, noteID int64) ([]int64, error)
	DecrementCounts(ctx context.Context, topicIDs []int64) error
	GetByID(ctx context.Context, id int64) (*model.Topic, error)
	Hot(ctx context.Context, limit int) ([]*model.Topic, error)
	Suggest(ctx context.Context, q string, limit int) ([]*model.Topic, error)
}

type GormTopicRepo struct {
	db *gorm.DB
}

func NewGormTopicRepo(db *gorm.DB) *GormTopicRepo {
	return &GormTopicRepo{db: db}
}

// UpsertByName 批量插入话题（存在则 note_count+1），返回 name -> id
func (r *GormTopicRepo) UpsertByName(ctx context.Context, names []string) (map[string]int64, error) {
	idByName := make(map[string]int64, len(names))
	for _, name := range names {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"note_count": gorm.Expr("note_count + 1")}),
		}).Create(&model.Topic{Name: name, NoteCount: 1}).Error; err != nil {
			return nil, err
		}
		var t model.Topic
		if err := r.db.WithContext(ctx).Select("id").Where("name = ?", name).First(&t).Error; err != nil {
			return nil, err
		}
		idByName[name] = t.ID
	}
	return idByName, nil
}

func (r *GormTopicRepo) AttachNoteTopics(ctx context.Context, noteID int64, topicIDs []int64) error {
	if len(topicIDs) == 0 {
		return nil
	}
	rows := make([]model.NoteTopic, 0, len(topicIDs))
	for _, tid := range topicIDs {
		rows = append(rows, model.NoteTopic{NoteID: noteID, TopicID: tid})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// DetachNoteTopics 删除笔记的全部话题关联，返回被解除的话题 id（调用方负责递减计数）
func (r *GormTopicRepo) DetachNoteTopics(ctx context.Context, noteID int64) ([]int64, error) {
	var ids []int64
	if err := r.db.WithContext(ctx).Model(&model.NoteTopic{}).
		Where("note_id = ?", noteID).Pluck("topic_id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		if err := r.db.WithContext(ctx).Where("note_id = ?", noteID).Delete(&model.NoteTopic{}).Error; err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (r *GormTopicRepo) DecrementCounts(ctx context.Context, topicIDs []int64) error {
	if len(topicIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.Topic{}).
		Where("id IN ?", topicIDs).
		UpdateColumn("note_count", gorm.Expr("GREATEST(note_count - 1, 0)")).Error
}

func (r *GormTopicRepo) GetByID(ctx context.Context, id int64) (*model.Topic, error) {
	var t model.Topic
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *GormTopicRepo) Hot(ctx context.Context, limit int) ([]*model.Topic, error) {
	var topics []*model.Topic
	err := r.db.WithContext(ctx).Order("note_count DESC, id ASC").Limit(limit).Find(&topics).Error
	return topics, err
}

func (r *GormTopicRepo) Suggest(ctx context.Context, q string, limit int) ([]*model.Topic, error) {
	var topics []*model.Topic
	err := r.db.WithContext(ctx).
		Where("name LIKE ?", "%"+q+"%").
		Order("note_count DESC, id ASC").
		Limit(limit).Find(&topics).Error
	return topics, err
}
