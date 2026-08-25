package repo

import (
	"XFeedSystem/internal/model"
	"context"
	"time"

	"gorm.io/gorm"
)

type ReportRepo interface {
	Create(ctx context.Context, r *model.Report) error
	GetByReporterTarget(ctx context.Context, reporterID, targetType, targetID int64) (*model.Report, error)
	GetByID(ctx context.Context, id int64) (*model.Report, error)
	ListByStatus(ctx context.Context, status, cursor int64, limit int) ([]*model.Report, int64, error)
	CountTodayByReporter(ctx context.Context, reporterID int64) (int64, error)
	UpdateStatus(ctx context.Context, id, status, handledBy int64) error
	ResetForReporter(ctx context.Context, r *model.Report) error // 重新举报时重置为待处理
	DeleteBothSide(ctx context.Context, messageID int64) error   // 私信双删（管理员处置）
}

type GormReportRepo struct{ db *gorm.DB }

func NewGormReportRepo(db *gorm.DB) *GormReportRepo { return &GormReportRepo{db: db} }

func (r *GormReportRepo) Create(ctx context.Context, rep *model.Report) error {
	return r.db.WithContext(ctx).
		Create(rep).Error
}

func (r *GormReportRepo) GetByReporterTarget(ctx context.Context, reporterID, targetType, targetID int64) (*model.Report, error) {
	var rep model.Report
	err := r.db.WithContext(ctx).
		Where("reporter_id = ? AND target_type = ? AND target_id = ?", reporterID, targetType, targetID).
		First(&rep).Error
	return &rep, err
}

func (r *GormReportRepo) GetByID(ctx context.Context, id int64) (*model.Report, error) {
	var rep model.Report
	if err := r.db.WithContext(ctx).
		First(&rep, id).Error; err != nil {
		return nil, err
	}
	return &rep, nil
}

func (r *GormReportRepo) ListByStatus(ctx context.Context, status, cursor int64, limit int) ([]*model.Report, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if cursor == 0 {
		cursor = 1<<63 - 1
	}
	var list []*model.Report
	err := r.db.WithContext(ctx).
		Where("status = ? AND id < ?", status, cursor).
		Order("id DESC").
		Limit(limit).
		Find(&list).Error
	var next int64
	if len(list) > 0 {
		next = list[len(list)-1].ID
	}
	return list, next, err
}

func (r *GormReportRepo) CountTodayByReporter(ctx context.Context, reporterID int64) (int64, error) {
	var n int64
	today := time.Now().Format("2006-01-02")
	err := r.db.WithContext(ctx).
		Model(&model.Report{}).
		Where("reporter_id = ? AND DATE(created_at) = ?", reporterID, today).
		Count(&n).Error
	return n, err
}

func (r *GormReportRepo) UpdateStatus(ctx context.Context, id, status, handledBy int64) error {
	return r.db.WithContext(ctx).Model(&model.Report{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"handled_by": handledBy,
			"handled_at": gorm.Expr("NOW(3)"),
		}).Error
}

func (r *GormReportRepo) ResetForReporter(ctx context.Context, rep *model.Report) error {
	return r.db.WithContext(ctx).Model(&model.Report{}).
		Where("id = ?", rep.ID).
		Updates(map[string]interface{}{
			"reason":          rep.Reason,
			"description":     rep.Description,
			"target_snapshot": rep.TargetSnapshot,
			"status":          model.ReportStatusPending,
			"handled_by":      0,
			"handled_at":      nil,
		}).Error
}

func (r *GormReportRepo) DeleteBothSide(ctx context.Context, messageID int64) error {
	return r.db.WithContext(ctx).Model(&model.Message{}).
		Where("id = ?", messageID).
		Updates(map[string]interface{}{"sender_deleted": 1, "receiver_deleted": 1}).Error
}
