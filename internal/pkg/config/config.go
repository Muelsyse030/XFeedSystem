package config

import (
	"os"
	"strings"

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

	// 环境变量覆盖 yaml 配置
	// 例如 XFEED_JWT_SECRET 覆盖 jwt.secret，XFEED_MYSQL_DSN 覆盖 mysql.dsn
	viper.SetEnvPrefix("XFEED")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// 绑定关键配置项到环境变量
	viper.BindEnv("mysql.dsn")
	viper.BindEnv("redis.addr")
	viper.BindEnv("redis.password")
	viper.BindEnv("jwt.secret")
	viper.BindEnv("jwt.expire_hours")
	viper.BindEnv("oss.enable")
	viper.BindEnv("oss.endpoint")
	viper.BindEnv("oss.bucket")
	viper.BindEnv("oss.access_key_id")
	viper.BindEnv("oss.access_key_secret")
	viper.BindEnv("oss.base_url")
	viper.BindEnv("meilisearch.host")
	viper.BindEnv("meilisearch.api_key")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}
	return &config, nil
}
