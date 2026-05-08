package config

import (
	"fmt"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"SERVER"`
	Postgres PostgresConfig `mapstructure:"POSTGRES"`
	Redis    RedisConfig    `mapstructure:"REDIS"`
}

type ServerConfig struct {
	Host string `mapstructure:"HOST"`
	Port int    `mapstructure:"PORT"`
	Env  string `mapstructure:"ENV"`
}

type PostgresConfig struct {
	Host     string `mapstructure:"HOST"`
	User     string `mapstructure:"USER"`
	Password string `mapstructure:"PASSWORD"`
	Database string `mapstructure:"DATABASE"`
	Port     int    `mapstructure:"PORT"`
}

type RedisConfig struct {
	Host     string `mapstructure:"HOST"`
	Port     int    `mapstructure:"PORT"`
	Password string `mapstructure:"PASSWORD"`
}

func Load(configPath string) (Config, error) {
	var cfg Config

	k := koanf.New(".")

	_ = k.Load(env.Provider("", "_", nil), nil)

	_ = k.Load(
		file.Provider(filepath.Join(configPath, ".env")),
		dotenv.ParserEnv("", "_", func(s string) string { return s }),
	)

	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		Tag: "mapstructure",
		DecoderConfig: &mapstructure.DecoderConfig{
			Result: &cfg,
			Squash: true,
		},
	}); err != nil {
		return cfg, fmt.Errorf("config parse failed: %w", err)
	}

	applyDefaults(&cfg)

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Env == "" {
		cfg.Server.Env = "production"
	}

	if cfg.Postgres.Port == 0 {
		cfg.Postgres.Port = 5432
	}

	if cfg.Redis.Port == 0 {
		cfg.Redis.Port = 6379
	}
}
