package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/erfanmomeniii/task-pipeline/internal/config"
	"github.com/erfanmomeniii/task-pipeline/internal/consumer"
	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/logger"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	pb "github.com/erfanmomeniii/task-pipeline/proto"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print build version and exit")
	cfgPath := flag.String("config", "", "path to config file")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if err := run(*cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
	metricsSrv := metrics.NewServer(cfg.Consumer.PrometheusPort, log)
	go func() {
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server error", "error", err)
		}
	}()

	// pprof
	pprofAddr := fmt.Sprintf(":%d", cfg.Consumer.PprofPort)
	log.Info("starting pprof server", "addr", pprofAddr)
	pprofSrv := &http.Server{Addr: pprofAddr, Handler: nil}
	go func() {
		if err := pprofSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
	c := consumer.New(queries, cfg.Consumer.RateLimit, cfg.Consumer.RatePeriodMs, cfg.Consumer.MaxWorkers, log)
	pb.RegisterTaskServiceServer(srv, c)

	// Periodically update tasks_by_state gauge from DB (every 5s).
	go c.StartStateTracker(ctx, 5*time.Second)

	log.Info("consumer starting",
		"version", version,
		"grpc_addr", grpcAddr,
		"rate_limit", cfg.Consumer.RateLimit,
		"rate_period_ms", cfg.Consumer.RatePeriodMs,
		"max_workers", cfg.Consumer.MaxWorkers,
	)

	// Graceful shutdown: stop accepting new RPCs, drain tasks, shut down HTTP servers.
	go func() {
		<-ctx.Done()
		log.Info("shutting down gRPC server")
		srv.GracefulStop()
		log.Info("waiting for in-flight tasks to complete")
		c.Wait()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		metricsSrv.Shutdown(shutdownCtx)
		pprofSrv.Shutdown(shutdownCtx)

		log.Info("all tasks completed, shutdown done")
	}()

	if err := srv.Serve(lis); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}

	return nil
}
