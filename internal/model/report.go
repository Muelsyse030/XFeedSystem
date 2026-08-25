package model

import "time"

const (
	ReportTargetNote    = 1
	ReportTargetComment = 2
	ReportTargetUser    = 3
	ReportTargetMessage = 4

	ReportStatusPending  = 0
	ReportStatusApproved = 1
	ReportStatusRejected = 2

	ReportReasonGarbage = 1 // 垃圾广告
	ReportReasonPorn    = 2 // 色情低俗
	ReportReasonAbuse   = 3 // 人身攻击/辱骂
	ReportReasonIllegal = 4 // 违法违规
	ReportReasonFake    = 5 // 虚假信息
	ReportReasonOther   = 6 // 其他
)

type Report struct {
	ID             int64      `gorm:"primaryKey" json:"id"`
	ReporterID     int64      `gorm:"not null;index" json:"reporter_id"`
	TargetType     int8       `gorm:"not null" json:"target_type"`
	TargetID       int64      `gorm:"not null" json:"target_id"`
	Reason         int8       `gorm:"not null" json:"reason"`
	Description    string     `gorm:"size:500;not null;default:''" json:"description"`
	TargetSnapshot string     `gorm:"type:text" json:"target_snapshot"`
	Status         int8       `gorm:"not null;default:0" json:"status"`
	HandledBy      int64      `gorm:"not null;default:0" json:"handled_by"`
	HandledAt      *time.Time `json:"handled_at"`
	CreatedAt      time.Time   `json:"created_at"`
}

func (Report) TableName() string { return "reports" }
