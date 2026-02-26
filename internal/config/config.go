package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Database     DatabaseConfig     `yaml:"database"`
	Log          LogConfig          `yaml:"log"`
	Metrics      MetricsConfig      `yaml:"metrics"`
	Scheduler    SchedulerConfig    `yaml:"scheduler"`
	LogRetention LogRetentionConfig `yaml:"log_retention"`
	Auth         AuthConfig         `yaml:"auth"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	URI             string `yaml:"uri"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type SchedulerConfig struct {
	Enabled bool `yaml:"enabled"`
}

type LogRetentionConfig struct {
	Days int `yaml:"days"`
}

type AuthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Secret   string `yaml:"secret"`
	MaxAge   int    `yaml:"max_age"`
}

// LoadConfig loads configuration from .env, config.yaml, and environment variables.
// Priority: environment variables > yaml file > defaults.
func LoadConfig() (*Config, error) {
	// Load .env file (ignore error if file doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{}

	// Read and parse config.yaml
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Environment variable overrides
	if v := os.Getenv("DATABASE_URI"); v != "" {
		cfg.Database.URI = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}

	// Auth defaults
	if cfg.Auth.MaxAge == 0 {
		cfg.Auth.MaxAge = 604800 // 7 days
	}

	return cfg, nil
}
