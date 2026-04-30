package main

import (
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"time"

	"google.golang.org/grpc"

	"github.com/erfanmomeniii/task-pipeline/internal/app"
	"github.com/erfanmomeniii/task-pipeline/internal/consumer"
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
	a, err := app.New(cfgPath)
	if err != nil {
		return err
	}

	a.WithLogger()
	a.WithGracefulShutdown()

	if err := a.WithDatabase(true); err != nil {
		return err
	}

	metrics.RegisterConsumer()
	a.WithMetrics(a.Cfg.Consumer.PrometheusPort)
	a.WithPprof(a.Cfg.Consumer.PprofPort)
	defer a.Shutdown(5 * time.Second)

	c := consumer.New(a.Queries, a.Cfg.Consumer.RateLimit, a.Cfg.Consumer.RatePeriodMs, a.Cfg.Consumer.MaxWorkers, a.Log)

	grpcAddr := fmt.Sprintf(":%d", a.Cfg.GRPC.Port)
	if err := a.WithGRPCServer(grpcAddr, func(srv *grpc.Server) {
		pb.RegisterTaskServiceServer(srv, c)
	}); err != nil {
		return err
	}

	// Periodically update tasks_by_state gauge from DB (every 5s).
	go c.StartStateTracker(a.Ctx, 5*time.Second)

	a.Log.Info("consumer starting",
		"version", version,
		"grpc_addr", grpcAddr,
		"rate_limit", a.Cfg.Consumer.RateLimit,
		"rate_period_ms", a.Cfg.Consumer.RatePeriodMs,
		"max_workers", a.Cfg.Consumer.MaxWorkers,
	)

	// Graceful shutdown: stop accepting new RPCs, then drain in-flight tasks.
	go func() {
		<-a.Ctx.Done()
		a.Log.Info("shutting down gRPC server")
		a.GracefulStopGRPC()
		a.Log.Info("waiting for in-flight tasks to complete")
		c.Wait()
		a.Log.Info("all tasks completed, shutdown done")
	}()

	return a.ServeGRPC()
}
