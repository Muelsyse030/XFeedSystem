package handler

import (
	"XFeedSystem/internal/service"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ReportHandler struct {
	reportService *service.ReportService
}

type CreateReportRequest struct {
	TargetType  int    `json:"target_type"`
	TargetID    int64  `json:"target_id"`
	Reason      int    `json:"reason"`
	Description string `json:"description"`
}

func NewReportHandler(s *service.ReportService) *ReportHandler {
	return &ReportHandler{reportService: s}
}

func (h *ReportHandler) Create(c *gin.Context) {
	meID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 4010, "message": "unauthorized"})
		return
	}
	var req CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TargetID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4001, "message": "invalid request"})
		return
	}
	rep, err := h.reportService.Report(c.Request.Context(), meID, int64(req.TargetType), req.TargetID, req.Reason, req.Description)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrAlreadyReported) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"code": 4002, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"id": rep.ID, "status": rep.Status}})
}

// 管理端
func (h *ReportHandler) List(c *gin.Context) { // /admin/reports?status=0
	status, _ := strconv.ParseInt(c.DefaultQuery("status", "0"), 10, 64)
	cursor, _ := strconv.ParseInt(c.DefaultQuery("cursor", "0"), 10, 64)
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	list, next, err := h.reportService.ListByStatus(c.Request.Context(), status, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"list": list, "next_cursor": next}})
}

func (h *ReportHandler) Handle(c *gin.Context) { // /admin/reports/:id  {action:"approve"|"reject"}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	adminID, _ := getUserIDFromContext(c)
	var req struct {
		Action string `json:"action"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.reportService.Handle(c.Request.Context(), adminID, id, req.Action == "approve"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}
