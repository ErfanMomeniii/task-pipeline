package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/erfanmomeniii/task-pipeline/internal/config"
	"github.com/erfanmomeniii/task-pipeline/internal/db"
	"github.com/erfanmomeniii/task-pipeline/internal/logger"
	"github.com/erfanmomeniii/task-pipeline/internal/metrics"
)

// App holds shared infrastructure resources used by both services.
// Builder methods initialize resources incrementally, keeping cmd/ thin.
type App struct {
	Cfg     *config.Config
	Log     *slog.Logger
	Pool    *pgxpool.Pool
	Queries *db.Queries
	Ctx     context.Context
	Cancel  context.CancelFunc

	grpcSrv    *grpc.Server
	grpcLis    net.Listener
	grpcConn   *grpc.ClientConn
	metricsSrv *http.Server
	pprofSrv   *http.Server
}

// New loads configuration and returns a partially initialized App.
func New(cfgPath string) (*App, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &App{Cfg: cfg}, nil
}

// WithLogger initializes the slog logger from config settings.
func (a *App) WithLogger() {
	a.Log = logger.New(os.Stdout, a.Cfg.Log.Format, a.Cfg.Log.Level)
}

// WithGracefulShutdown creates a context that cancels on SIGINT or SIGTERM.
func (a *App) WithGracefulShutdown() {
	a.Ctx, a.Cancel = signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

// WithDatabase connects to PostgreSQL and optionally runs migrations.
func (a *App) WithDatabase(migrate bool) error {
	dsn := db.DSN(a.Cfg.DB.Host, a.Cfg.DB.Port, a.Cfg.DB.User, a.Cfg.DB.Password, a.Cfg.DB.Name, a.Cfg.DB.SSLMode)

	if migrate {
		if err := db.Migrate(dsn, a.Log); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	pool, queries, err := db.Connect(a.Ctx, dsn, a.Log)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}

	a.Pool = pool
	a.Queries = queries
	return nil
}

// WithMetrics starts a Prometheus /metrics HTTP server on the given port.
func (a *App) WithMetrics(port int) {
	a.metricsSrv = metrics.NewServer(port, a.Log)
	go func() {
		if err := a.metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.Log.Error("metrics server error", "error", err)
		}
	}()
}

// WithPprof starts a pprof HTTP server on the given port.
// Uses a dedicated ServeMux with explicitly registered handlers instead of
// relying on the DefaultServeMux via blank import, avoiding global side effects.
func (a *App) WithPprof(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	pprofAddr := fmt.Sprintf(":%d", port)
	a.Log.Info("starting pprof server", "addr", pprofAddr)
	a.pprofSrv = &http.Server{Addr: pprofAddr, Handler: mux}
	go func() {
		if err := a.pprofSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.Log.Error("pprof server error", "error", err)
		}
	}()
}

// WithGRPCServer creates a gRPC server and listener on the given address.
// The register callback is invoked to register service implementations before serving.
func (a *App) WithGRPCServer(addr string, register func(srv *grpc.Server)) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	a.grpcSrv = grpc.NewServer()
	a.grpcLis = lis
	register(a.grpcSrv)
	return nil
}

// ServeGRPC starts serving gRPC requests. It blocks until the server stops.
func (a *App) ServeGRPC() error {
	if err := a.grpcSrv.Serve(a.grpcLis); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}

// GracefulStopGRPC gracefully stops the gRPC server, blocking until all
// in-flight RPCs complete.
func (a *App) GracefulStopGRPC() {
	if a.grpcSrv != nil {
		a.grpcSrv.GracefulStop()
	}
}

// WithGRPCClient dials a gRPC connection to the given address.
// The connection is closed automatically by Shutdown.
func (a *App) WithGRPCClient(addr string) error {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	a.grpcConn = conn
	return nil
}

// GRPCConn returns the gRPC client connection established by WithGRPCClient.
func (a *App) GRPCConn() *grpc.ClientConn {
	return a.grpcConn
}

// Shutdown gracefully stops HTTP servers, gRPC client, and database pool.
func (a *App) Shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if a.metricsSrv != nil {
		if err := a.metricsSrv.Shutdown(ctx); err != nil {
			a.Log.Error("metrics server shutdown error", "error", err)
		}
	}
	if a.pprofSrv != nil {
		if err := a.pprofSrv.Shutdown(ctx); err != nil {
			a.Log.Error("pprof server shutdown error", "error", err)
		}
	}
	if a.grpcConn != nil {
		if err := a.grpcConn.Close(); err != nil {
			a.Log.Error("grpc client close error", "error", err)
		}
	}
	if a.Pool != nil {
		a.Pool.Close()
	}
}
