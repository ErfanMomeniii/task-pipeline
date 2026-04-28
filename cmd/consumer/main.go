package main

import (
	"fmt"
	"os"

	"github.com/erfanmomeniii/task-pipeline/internal/config"
	"github.com/erfanmomeniii/task-pipeline/internal/logger"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	var cfgPath string

	root := &cobra.Command{
		Use:   "consumer",
		Short: "Task Pipeline — Consumer service",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			log := logger.New(os.Stdout, cfg.Log.Format, cfg.Log.Level)
			log.Info("consumer starting",
				"version", version,
				"grpc_addr", fmt.Sprintf("%s:%d", cfg.GRPC.Host, cfg.GRPC.Port),
				"rate_limit", cfg.Consumer.RateLimit,
				"rate_period_ms", cfg.Consumer.RatePeriodMs,
			)

			// TODO: wire consumer business logic
			return nil
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
