package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"XFeedSystem/internal/pkg/config"
	"XFeedSystem/internal/pkg/logger"

	"github.com/google/uuid"
	"github.com/tencentyun/cos-go-sdk-v5"
)

var ErrCOSDisabled = errors.New("cos is disabled")

type StorageService struct {
	cfg    config.Config
	client *cos.Client
}

func NewStorageService(cfg config.Config) (*StorageService, error) {
	if !cfg.COS.Enable {
		return &StorageService{cfg: cfg}, nil
	}
	if cfg.COS.Region == "" || cfg.COS.Bucket == "" || cfg.COS.SecretID == "" || cfg.COS.SecretKey == "" {
		logger.Sugar.Warnf("[WARN] COS 配置不完整，已自动禁用图片上传功能。请配置 XFEED_COS_* 环境变量后重启。")
		cfg.COS.Enable = false
		return &StorageService{cfg: cfg}, nil
	}
	u, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cfg.COS.Bucket, cfg.COS.Region))
	if err != nil {
		return nil, err
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.COS.SecretID,
			SecretKey: cfg.COS.SecretKey,
		},
	})
	return &StorageService{cfg: cfg, client: client}, nil
}

func (s *StorageService) UploadImage(ctx context.Context, file multipart.File, header *multipart.FileHeader, prefix string) (string, error) {
	if !s.cfg.COS.Enable || s.client == nil {
		return "", ErrCOSDisabled
	}
	defer file.Close()
	data, err := ioReadAll(file)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".png"
	}
	objectKey := fmt.Sprintf("%s/%s%s", strings.Trim(prefix, "/"), uuid.NewString(), ext)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if _, err := s.client.Object.Put(ctx, objectKey, bytes.NewReader(data), &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentType: contentType},
		ACLHeaderOptions:       &cos.ACLHeaderOptions{XCosACL: "public-read"},
	}); err != nil {
		return "", err
	}
	if s.cfg.COS.BaseURL != "" {
		return strings.TrimRight(s.cfg.COS.BaseURL, "/") + "/" + objectKey, nil
	}
	return objectKey, nil
}

func ioReadAll(file multipart.File) ([]byte, error) {
	return io.ReadAll(file)
}

func (s *StorageService) UploadVideo(ctx context.Context, file multipart.File, header *multipart.FileHeader, prefix string) (string, error) {
	if !s.cfg.COS.Enable || s.client == nil {
		return "", ErrCOSDisabled
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".mp4"
	}
	objectKey := fmt.Sprintf("%s/%s%s", strings.Trim(prefix, "/"), uuid.NewString(), ext)
	if _, err := s.client.Object.Put(ctx, objectKey, file, &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentType: header.Header.Get("Content-Type")},
		ACLHeaderOptions:       &cos.ACLHeaderOptions{XCosACL: "public-read"},
	}); err != nil {
		return "", err
	}
	return strings.TrimRight(s.cfg.COS.BaseURL, "/") + "/" + objectKey, nil
}
