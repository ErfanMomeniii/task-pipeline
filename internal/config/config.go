package config

import (
	"errors"
	"fmt"
)

// Config holds the shared configuration for both services.
// Each service loads only the fields it needs.
type Config struct {
	DB       DBConfig       `mapstructure:"db"`
	Log      LogConfig      `mapstructure:"log"`
	GRPC     GRPCConfig     `mapstructure:"grpc"`
	Producer ProducerConfig `mapstructure:"producer"`
	Consumer ConsumerConfig `mapstructure:"consumer"`
}

type DBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	SSLMode  string `mapstructure:"sslmode"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type GRPCConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type ProducerConfig struct {
	PrometheusPort int `mapstructure:"prometheus_port"`
	PprofPort      int `mapstructure:"pprof_port"`
	RatePerSecond  int `mapstructure:"rate_per_second"`
	MaxBacklog     int `mapstructure:"max_backlog"`
}

type ConsumerConfig struct {
	PrometheusPort int `mapstructure:"prometheus_port"`
	PprofPort      int `mapstructure:"pprof_port"`
	RateLimit      int `mapstructure:"rate_limit"`
	RatePeriodMs   int `mapstructure:"rate_period_ms"`
	MaxWorkers     int `mapstructure:"max_workers"`
}

// Validate checks that all configuration values are within acceptable ranges.
func (c *Config) Validate() error {
	var errs []error

	checkPort := func(name string, port int) {
		if port < 1 || port > 65535 {
			errs = append(errs, fmt.Errorf("%s: port %d out of range [1, 65535]", name, port))
		}
	}

	checkPort("db.port", c.DB.Port)
	checkPort("grpc.port", c.GRPC.Port)
	checkPort("producer.prometheus_port", c.Producer.PrometheusPort)
	checkPort("producer.pprof_port", c.Producer.PprofPort)
	checkPort("consumer.prometheus_port", c.Consumer.PrometheusPort)
	checkPort("consumer.pprof_port", c.Consumer.PprofPort)

	if c.Producer.RatePerSecond <= 0 {
		errs = append(errs, fmt.Errorf("producer.rate_per_second: must be > 0, got %d", c.Producer.RatePerSecond))
	}
	if c.Producer.MaxBacklog <= 0 {
		errs = append(errs, fmt.Errorf("producer.max_backlog: must be > 0, got %d", c.Producer.MaxBacklog))
	}
	if c.Consumer.RateLimit <= 0 {
		errs = append(errs, fmt.Errorf("consumer.rate_limit: must be > 0, got %d", c.Consumer.RateLimit))
	}

	return errors.Join(errs...)
}
