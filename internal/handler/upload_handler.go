package handler

import (
	"XFeedSystem/internal/pkg/logger"
	"XFeedSystem/internal/service"
	"net/http"
	"path/filepath"
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
		contentType = inferImageContentType(header.Filename)
		if contentType == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 4002, "message": "only image files are allowed"})
			return
		}
		header.Header.Set("Content-Type", contentType)
	} else if contentType == "" {
		contentType = inferImageContentType(header.Filename)
		if contentType != "" {
			header.Header.Set("Content-Type", contentType)
		}
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

func inferImageContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	fallbackByExt := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".heic": "image/heic",
		".heif": "image/heif",
		".bmp":  "image/bmp",
	}

	if inferred, ok := fallbackByExt[ext]; ok {
		return inferred
	}

	// 没有可识别的扩展名时，不把 octet-stream 当作图片。
	return ""
}

func (h *UploadHandler) Video(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"code": 4001, "message": "file is required"})
		return
	}
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "video/") {
		contentType = inferVideoContentType(header.Filename)
		if contentType == "" {
			c.JSON(400, gin.H{"code": 4002, "message": "only video files are allowed"})
			return
		}
		header.Header.Set("Content-Type", contentType)
	}
	if header.Size > 200<<20 { // 200MB 上限
		c.JSON(400, gin.H{"code": 4003, "message": "video too large"})
		return
	}
	url, err := h.storageService.UploadVideo(c.Request.Context(), file, header, "videos")
	if err != nil {
		c.JSON(500, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": gin.H{"url": url}})
}

func inferVideoContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	}
	return ""
}
