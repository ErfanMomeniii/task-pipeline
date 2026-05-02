package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/erfanmomeniii/task-pipeline/internal/app"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
	"github.com/erfanmomeniii/task-pipeline/internal/producer"
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

	// Database — migrations are owned by the consumer service to avoid
	// race conditions when both services start simultaneously.
	if err := a.WithDatabase(false); err != nil {
		return err
	}

	metrics.RegisterProducer()
	a.WithMetrics(a.Cfg.Producer.PrometheusPort)
	a.WithPprof(a.Cfg.Producer.PprofPort)
	defer a.Shutdown(5 * time.Second)

	grpcAddr := fmt.Sprintf("%s:%d", a.Cfg.GRPC.Host, a.Cfg.GRPC.Port)
	if err := a.WithGRPCClient(grpcAddr); err != nil {
		return err
	}

	client := pb.NewTaskServiceClient(a.GRPCConn())
	p := producer.New(client, a.Queries, a.Cfg.Producer.RatePerSecond, a.Cfg.Producer.MaxBacklog, a.Log)

	a.Log.Info("producer starting",
		"version", version,
		"grpc_addr", grpcAddr,
		"rate_per_second", a.Cfg.Producer.RatePerSecond,
		"max_backlog", a.Cfg.Producer.MaxBacklog,
	)

	return p.Run(a.Ctx)
}
