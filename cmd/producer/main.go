package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/erfanmomeniii/task-pipeline/internal/config"
	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/logger"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	"github.com/erfanmomeniii/task-pipeline/internal/producer"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	var cfgPath string

	root := &cobra.Command{
		Use:   "producer",
		Short: "Task Pipeline — Producer service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cfgPath)
		},
	}

	root.Flags().StringVar(&cfgPath, "config", "", "path to config file")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print build version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(os.Stdout, cfg.Log.Format, cfg.Log.Level)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Database
	dsn := db.DSN(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode)
	if err := db.Migrate(dsn, log); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	pool, queries, err := db.Connect(ctx, dsn, log)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()

	// Prometheus metrics
	metrics.RegisterProducer()
	go func() {
		if err := metrics.Serve(cfg.Producer.PrometheusPort, log); err != nil {
			log.Error("metrics server error", "error", err)
		}
	}()

	// pprof
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Producer.PprofPort)
		log.Info("starting pprof server", "addr", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Error("pprof server error", "error", err)
		}
	}()

	// Producer
	grpcAddr := fmt.Sprintf("%s:%d", cfg.GRPC.Host, cfg.GRPC.Port)
	p, err := producer.New(ctx, queries, grpcAddr, cfg.Producer.RateMs, cfg.Producer.MaxBacklog, log)
	if err != nil {
		return fmt.Errorf("create producer: %w", err)
	}
	defer p.Close()

	log.Info("producer starting",
		"version", version,
		"grpc_addr", grpcAddr,
		"rate_ms", cfg.Producer.RateMs,
		"max_backlog", cfg.Producer.MaxBacklog,
	)

	return p.Run(ctx)
}
