package handler

import (
	"XFeedSystem/internal/pkg/logger"
	"XFeedSystem/internal/service"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type UploadHandler struct {
	storageService *service.StorageService
}

func NewUploadHandler(storageService *service.StorageService) *UploadHandler {
	return &UploadHandler{storageService: storageService}
}

func (h *UploadHandler) Image(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4001, "message": "file is required"})
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4002, "message": "only image files are allowed"})
		return
	}

	url, err := h.storageService.UploadImage(c.Request.Context(), file, header, "images")
	if err != nil {
		logger.Sugar.Errorf("upload image failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}

	logger.Sugar.Infof("upload image success, url: %s, filename: %s, size: %d", url, header.Filename, header.Size)
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    gin.H{"url": url},
	})
}
