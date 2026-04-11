package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Email  EmailConfig  `mapstructure:"email"`
	Server ServerConfig `mapstructure:"server"`
	TLS    TLSConfig    `mapstructure:"tls"`
	Auth   AuthConfig   `mapstructure:"auth"`
}

type EmailConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type TLSConfig struct {
	Enable bool   `mapstructure:"enable"`
	Key    string `mapstructure:"key"`
	Cert   string `mapstructure:"cert"`
}

type AuthConfig struct {
	Key string `mapstructure:"key"`
}

func New() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config error: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
	}

	return &cfg, nil
}
