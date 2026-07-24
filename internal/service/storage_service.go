package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"path/filepath"
	"strings"

	"XFeedSystem/internal/pkg/config"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/google/uuid"
)

var ErrOSSDisabled = errors.New("oss is disabled")

type StorageService struct {
	cfg    config.Config
	client *oss.Client
	bucket *oss.Bucket
}

func NewStorageService(cfg config.Config) (*StorageService, error) {
	if !cfg.OSS.Enable {
		return &StorageService{cfg: cfg}, nil
	}
	if cfg.OSS.Endpoint == "" || cfg.OSS.Bucket == "" || cfg.OSS.AccessKeyID == "" || cfg.OSS.AccessKeySecret == "" {
		log.Printf("[WARN] OSS 配置不完整，已自动禁用图片上传功能。请配置 XFEED_OSS_* 环境变量后重启。")
		cfg.OSS.Enable = false
		return &StorageService{cfg: cfg}, nil
	}
	client, err := oss.New(cfg.OSS.Endpoint, cfg.OSS.AccessKeyID, cfg.OSS.AccessKeySecret)
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(cfg.OSS.Bucket)
	if err != nil {
		return nil, err
	}
	return &StorageService{cfg: cfg, client: client, bucket: bucket}, nil
}

func (s *StorageService) UploadImage(ctx context.Context, file multipart.File, header *multipart.FileHeader, prefix string) (string, error) {
	if !s.cfg.OSS.Enable || s.bucket == nil {
		return "", ErrOSSDisabled
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
	if err := s.bucket.PutObject(objectKey, bytes.NewReader(data)); err != nil {
		return "", err
	}
	if s.cfg.OSS.BaseURL != "" {
		return strings.TrimRight(s.cfg.OSS.BaseURL, "/") + "/" + objectKey, nil
	}
	return objectKey, nil
}

func ioReadAll(file multipart.File) ([]byte, error) {
	return io.ReadAll(file)
}
