package handler

import (
	"XFeedSystem/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	adminService *service.AdminService
}

func NewAdminHandler(as *service.AdminService) *AdminHandler {
	return &AdminHandler{adminService: as}
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	cursor, _ := strconv.ParseInt(c.DefaultQuery("cursor", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	keyword := c.Query("q")

	resp, err := h.adminService.ListUsers(c.Request.Context(), cursor, limit, keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": resp})
}

func (h *AdminHandler) BanUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid user id"})
		return
	}
	roleVal, _ := c.Get("role")
	opRole := roleVal.(int8)

	if err := h.adminService.BanUser(c.Request.Context(), opRole, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4004, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid user id"})
		return
	}
	roleVal, _ := c.Get("role")
	opRole := roleVal.(int8)

	if err := h.adminService.DeleteUser(c.Request.Context(), id, opRole); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4004, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

func (h *AdminHandler) DeleteNote(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid note id"})
		return
	}
	if err := h.adminService.DeleteNote(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4004, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

func (h *AdminHandler) DeleteComment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid comment id"})
		return
	}
	if err := h.adminService.DeleteComment(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4004, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

func (h *AdminHandler) Stats(c *gin.Context) {
	stats, err := h.adminService.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": stats})
}
