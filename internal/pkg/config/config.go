package config

import "github.com/spf13/viper"

type Config struct {
	Server struct {
		Port int
	}
	MySQL struct {
		DSN string
	}
	Redis struct {
		Addr     string
		Password string
		DB       int
	}
	JWT struct {
		Secret         string
		ExpireDuration int
	}
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}
	return &config, nil
}
