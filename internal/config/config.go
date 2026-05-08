// Package config loads runtime configuration from environment variables.
package config

import (
	"context"

	"github.com/sethvargo/go-envconfig"
)

// Config holds runtime configuration for the CLI.
type Config struct {
	AppEnv   string `env:"APP_ENV, default=development"`
	LogLevel string `env:"LOG_LEVEL, default=info"`
}

// Load reads configuration from the process environment.
func Load(ctx context.Context) (*Config, error) {
	var cfg Config
	if err := envconfig.Process(ctx, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
