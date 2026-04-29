package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/config"
	"github.com/erfanmomeniii/task-pipeline/internal/consumer"
	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/logger"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

var version = "dev"

func main() {
	// Spec requires: ./consumer -version
	if len(os.Args) > 1 && os.Args[1] == "-version" {
		fmt.Println(version)
		return
	}

	var cfgPath string

	root := &cobra.Command{
		Use:   "consumer",
		Short: "Task Pipeline — Consumer service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cfgPath)
		},
	}

	root.Flags().StringVar(&cfgPath, "config", "", "path to config file")

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
	metrics.RegisterConsumer()
	go func() {
		if err := metrics.Serve(cfg.Consumer.PrometheusPort, log); err != nil {
			log.Error("metrics server error", "error", err)
		}
	}()

	// pprof
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Consumer.PprofPort)
		log.Info("starting pprof server", "addr", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Error("pprof server error", "error", err)
		}
	}()

	// gRPC server
	grpcAddr := fmt.Sprintf(":%d", cfg.GRPC.Port)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", grpcAddr, err)
	}

	srv := grpc.NewServer()
	c := consumer.New(queries, cfg.Consumer.RateLimit, cfg.Consumer.RatePeriodMs, log)
	pb.RegisterTaskServiceServer(srv, c)

	// Periodically update tasks_by_state gauge from DB (every 5s).
	go c.StartStateTracker(ctx, 5*time.Second)

	log.Info("consumer starting",
		"version", version,
		"grpc_addr", grpcAddr,
		"rate_limit", cfg.Consumer.RateLimit,
		"rate_period_ms", cfg.Consumer.RatePeriodMs,
	)

	// Graceful shutdown: stop accepting new RPCs, then wait for in-flight tasks.
	go func() {
		<-ctx.Done()
		log.Info("shutting down gRPC server")
		srv.GracefulStop()
		log.Info("waiting for in-flight tasks to complete")
		c.Wait()
		log.Info("all tasks completed, shutdown done")
	}()

	if err := srv.Serve(lis); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}

	return nil
}
