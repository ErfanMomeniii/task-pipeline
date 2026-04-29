package config

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
