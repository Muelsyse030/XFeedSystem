package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server struct {
		Port int `mapstructure:"port"`
	} `mapstructure:"server"`
	MySQL struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"mysql"`
	Redis struct {
		Addr     string `mapstructure:"addr"`
		Password string `mapstructure:"password"`
		DB       int    `mapstructure:"db"`
	} `mapstructure:"redis"`
	JWT struct {
		Secret         string `mapstructure:"secret"`
		ExpireDuration int    `mapstructure:"expire_hours"`
	} `mapstructure:"jwt"`
	OSS struct {
		Enable          bool   `mapstructure:"enable"`
		Endpoint        string `mapstructure:"endpoint"`
		Bucket          string `mapstructure:"bucket"`
		AccessKeyID     string `mapstructure:"access_key_id"`
		AccessKeySecret string `mapstructure:"access_key_secret"`
		BaseURL         string `mapstructure:"base_url"`
	} `mapstructure:"oss"`
	Meilisearch struct {
		Host   string `mapstructure:"host"`
		APIKey string `mapstructure:"api_key"`
		Index  string `mapstructure:"index"`
	} `mapstructure:"meilisearch"`
	Streams struct {
		Key   string `mapstructure:"key"`
		Group string `mapstructure:"group"`
	} `mapstructure:"streams"` // 组名固定写在 events 包，这里只需 key
	Worker struct {
		RelayEnabled   bool `mapstructure:"relay_enabled"`
		Batch          int  `mapstructure:"batch"`
		PollIntervalMs int  `mapstructure:"poll_interval_ms"`
	} `mapstructure:"worker"`
}

func LoadConfig() (*Config, error) {
	// 配置路径：优先使用环境变量 CONFIG_PATH，默认 ./configs
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs"
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configPath)

	// 环境变量覆盖 yaml 配置（只对显式 SetEnvPrefix+BindEnv 的 key 生效）
	viper.SetEnvPrefix("XFEED")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.BindEnv("oss.access_key_id")
	viper.BindEnv("oss.access_key_secret")
	viper.BindEnv("jwt.secret")
	viper.BindEnv("server.port")
	viper.BindEnv("mysql.dsn")
	viper.BindEnv("redis.addr")
	viper.BindEnv("redis.password")
	viper.BindEnv("meilisearch.host")
	viper.BindEnv("meilisearch.api_key")
	viper.BindEnv("meilisearch.index")
	viper.BindEnv("streams.key")
	viper.BindEnv("worker.relay_enabled")
	viper.BindEnv("worker.batch")
	viper.BindEnv("worker.poll_interval_ms")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	// 从 .env 文件只读取 OSS 密钥（不影响其他任何配置）
	envMap := loadEnvFile(configPath)
	if ak, ok := envMap["XFEED_OSS_ACCESS_KEY_ID"]; ok && ak != "" {
		viper.Set("oss.access_key_id", ak)
	}
	if sk, ok := envMap["XFEED_OSS_ACCESS_KEY_SECRET"]; ok && sk != "" {
		viper.Set("oss.access_key_secret", sk)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

// loadEnvFile 查找并解析 .env 文件，只返回 map，不修改进程环境变量
func loadEnvFile(configPath string) map[string]string {
	candidates := []string{"/opt/xfeed/.env"}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, ".env"))
	}
	if configPath != "" {
		candidates = append(candidates, filepath.Join(configPath, "..", ".env"))
	}
	for _, f := range candidates {
		if m, err := godotenv.Read(f); err == nil {
			return m
		}
	}
	return nil
}
