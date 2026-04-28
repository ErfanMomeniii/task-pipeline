package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load reads configuration from the given file path and environment variables.
// Environment variables are prefixed with TP_ (task-pipeline) and use underscores
// to separate nested keys (e.g., TP_DB_HOST maps to db.host).
func Load(path string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("db.host", "localhost")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.user", "taskpipeline")
	v.SetDefault("db.password", "taskpipeline")
	v.SetDefault("db.name", "taskpipeline")
	v.SetDefault("db.sslmode", "disable")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")

	v.SetDefault("grpc.host", "localhost")
	v.SetDefault("grpc.port", 50051)

	v.SetDefault("producer.prometheus_port", 9090)
	v.SetDefault("producer.pprof_port", 6060)
	v.SetDefault("producer.rate_ms", 500)
	v.SetDefault("producer.max_backlog", 100)

	v.SetDefault("consumer.prometheus_port", 9091)
	v.SetDefault("consumer.pprof_port", 6061)
	v.SetDefault("consumer.rate_limit", 10)
	v.SetDefault("consumer.rate_period_ms", 1000)

	// File
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
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

	return &cfg, nil
}
