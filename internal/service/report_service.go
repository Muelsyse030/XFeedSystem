package service

import (
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const MaxDailyReports = 20

var (
	ErrInvalidReportTarget  = errors.New("举报对象不合法")
	ErrInvalidReportReason  = errors.New("举报原因不合法")
	ErrReportTargetMissing  = errors.New("举报对象不存在")
	ErrAlreadyReported      = errors.New("该内容你已举报，等待处理")
	ErrTooManyReports       = errors.New("今日举报次数已达上限")
	ErrReportNotFound       = errors.New("举报记录不存在")
	ErrReportAlreadyHandled = errors.New("该举报已处理")
)

type ReportService struct {
	repo        *repo.GormReportRepo
	noteRepo    *repo.GormNoteRepo
	userRepo    *repo.GormUserRepo
	messageRepo *repo.GormMessageRepo
	adminRepo   *repo.GormAdminRepo // 处置时复用删除/封禁
}

func NewReportService(r *repo.GormReportRepo, nr *repo.GormNoteRepo, ur *repo.GormUserRepo,
	mr *repo.GormMessageRepo, ar *repo.GormAdminRepo) *ReportService {
	return &ReportService{repo: r, noteRepo: nr, userRepo: ur, messageRepo: mr, adminRepo: ar}
}

func (s *ReportService) buildSnapshot(ctx context.Context, targetType, targetID int64) (string, error) {
	switch targetType {
	case model.ReportTargetNote:
		n, err := s.noteRepo.GetByID(ctx, targetID)
		if err != nil {
			return "", err
		}
		return "标题: " + n.Title + "\n内容: " + n.Content, nil
	case model.ReportTargetComment:
		c, err := s.noteRepo.GetCommentByID(ctx, targetID)
		if err != nil {
			return "", err
		}
		return "评论: " + c.Content, nil
	case model.ReportTargetUser:
		users, err := s.userRepo.GetByIDs([]int64{targetID})
		if err != nil || len(users) == 0 {
			return "", gorm.ErrRecordNotFound
		}
		return "用户名: " + users[0].Username, nil
	case model.ReportTargetMessage:
		m, err := s.messageRepo.GetByID(ctx, targetID)
		if err != nil {
			return "", err
		}
		return "私信: " + m.Content, nil
	}
	return "", ErrInvalidReportTarget
}

func (s *ReportService) Report(ctx context.Context, reporterID, targetType, targetID int64, reason int, description string) (*model.Report, error) {
	if targetType < model.ReportTargetNote || targetType > model.ReportTargetMessage {
		return nil, ErrInvalidReportTarget
	}
	if reason < model.ReportReasonGarbage || reason > model.ReportReasonOther {
		return nil, ErrInvalidReportReason
	}
	snapshot, err := s.buildSnapshot(ctx, targetType, targetID)
	if err != nil {
		return nil, ErrReportTargetMissing
	}
	if n, _ := s.repo.CountTodayByReporter(ctx, reporterID); n >= MaxDailyReports {
		return nil, ErrTooManyReports
	}

	rep := &model.Report{
		ReporterID:     reporterID,
		TargetType:     int8(targetType),
		TargetID:       targetID,
		Reason:         int8(reason),
		Description:    strings.TrimSpace(description),
		TargetSnapshot: snapshot,
		Status:         model.ReportStatusPending,
		CreatedAt:      time.Now(),
	}
	if err := s.repo.Create(ctx, rep); err != nil {
		if isDuplicateKey(err) {
			existing, gerr := s.repo.GetByReporterTarget(ctx, reporterID, targetType, targetID)
			if gerr != nil {
				return nil, gerr
			}
			if existing.Status == model.ReportStatusPending {
				return nil, ErrAlreadyReported
			}
			// 已处理过：允许重新举报，重置为待处理并更新快照
			existing.Reason = rep.Reason
			existing.Description = rep.Description
			existing.TargetSnapshot = snapshot
			if err := s.repo.ResetForReporter(ctx, existing); err != nil {
				return nil, err
			}
			return existing, nil
		}
		return nil, err
	}
	return rep, nil
}

func (s *ReportService) ListByStatus(ctx context.Context, status, cursor, limit int64) ([]*model.Report, int64, error) {
	return s.repo.ListByStatus(ctx, status, cursor, int(limit))
}

// Handle 管理员处置：成立→删除/封禁；驳回→标记。处置成功后才改举报状态。
func (s *ReportService) Handle(ctx context.Context, adminID, reportID int64, approve bool) error {
	rep, err := s.repo.GetByID(ctx, reportID)
	if err != nil {
		return ErrReportNotFound
	}
	if rep.Status != model.ReportStatusPending {
		return ErrReportAlreadyHandled
	}
	if !approve {
		return s.repo.UpdateStatus(ctx, rep.ID, model.ReportStatusRejected, adminID)
	}
	// 成立：先处置，成功后再标记
	switch rep.TargetType {
	case model.ReportTargetNote:
		if err := s.adminRepo.DeleteNoteByID(ctx, rep.TargetID); err != nil {
			return err
		}
	case model.ReportTargetComment:
		if err := s.adminRepo.DeleteCommentByID(ctx, rep.TargetID); err != nil {
			return err
		}
	case model.ReportTargetUser:
		if err := s.adminRepo.UpdateUserStatus(ctx, rep.TargetID, 0); err != nil { // 封禁
			return err
		}
	case model.ReportTargetMessage:
		if err := s.repo.DeleteBothSide(ctx, rep.TargetID); err != nil {
			return err
		}
	}
	return s.repo.UpdateStatus(ctx, rep.ID, model.ReportStatusApproved, adminID)
}
