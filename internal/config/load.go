package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// defaultConfig is the embedded default configuration used as a fallback
// when no config file is provided. Demonstrates use of the embed package
// beyond migrations.
//
//go:embed config.default.yaml
var defaultConfig []byte

// loadDefaults is the config bytes used as baseline. Defaults to the embedded
// config.default.yaml. Tests can override this to inject errors.
var loadDefaults = defaultConfig

// Load reads configuration from the given file path and environment variables.
// When no file path is provided, embedded default values are used.
// Environment variables are prefixed with TP_ (task-pipeline) and use underscores
// to separate nested keys (e.g., TP_DB_HOST maps to db.host).
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	// Load embedded defaults as baseline.
	if err := v.ReadConfig(bytes.NewReader(loadDefaults)); err != nil {
		return nil, fmt.Errorf("read embedded defaults: %w", err)
	}

	// Override with user-provided config file.
	if path != "" {
		v.SetConfigFile(path)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
	}

	// Environment: TP_DB_HOST -> db.host
	v.SetEnvPrefix("TP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}
