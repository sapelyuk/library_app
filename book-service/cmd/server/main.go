// Command server starts the Book Service gRPC server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	bookv1 "library_app/book-service/gen/go/book/v1"
	"library_app/book-service/internal/handler"
	"library_app/book-service/internal/repository/memory"
	"library_app/book-service/internal/service"
	pkgconfig "library_app/pkg/config"
	"library_app/pkg/logger"
)

// healthService matches the proto package of the API.
const healthService = "book.v1.BookService"

type appConfig struct {
	grpcAddr        string
	logLevel        string
	logFormat       string
	shutdownTimeout time.Duration
}

func loadConfig() appConfig {
	return appConfig{
		grpcAddr:        pkgconfig.String("BOOK_SERVICE_GRPC_ADDR", ":8081"),
		logLevel:        pkgconfig.String("BOOK_SERVICE_LOG_LEVEL", "info"),
		logFormat:       pkgconfig.String("BOOK_SERVICE_LOG_FORMAT", "json"),
		shutdownTimeout: pkgconfig.Duration("BOOK_SERVICE_SHUTDOWN_TIMEOUT", 15*time.Second),
	}
}

func main() {
	cfg := loadConfig()

	log, err := logger.New(logger.Options{Level: cfg.logLevel, Format: cfg.logFormat})
	if err != nil {
		slog.Error("book-service: bad logger configuration", "error", err)
		os.Exit(1)
	}

	if err := run(cfg, log); err != nil {
		log.Error("book-service stopped", "error", err)
		os.Exit(1)
	}

	log.Info("book-service stopped")
}

func run(cfg appConfig, log *slog.Logger) error {
	store := memory.NewStore()
	bookService := service.NewBookService(store, store)

	grpcServer := grpc.NewServer()
	bookv1.RegisterBookServiceServer(grpcServer, handler.NewGRPCServer(bookService, log))

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus(healthService, healthpb.HealthCheckResponse_SERVING)

	// Reflection keeps grpcurl and other generic clients usable during development.
	reflection.Register(grpcServer)

	listener, err := net.Listen("tcp", cfg.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.grpcAddr, err)
	}

	serveErr := make(chan error, 1)

	go func() {
		log.Info("book-service listening", "addr", cfg.grpcAddr, "storage", "in-memory")

		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErr <- err

			return
		}

		serveErr <- nil
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Info("shutdown signal received", "timeout", cfg.shutdownTimeout.String())

		shutdown(grpcServer, healthServer, cfg.shutdownTimeout, log)

		return nil
	}
}

func shutdown(server *grpc.Server, healthServer *health.Server, timeout time.Duration, log *slog.Logger) {
	// Stop advertising readiness first so that the gateway drains connections.
	healthServer.SetServingStatus(healthService, healthpb.HealthCheckResponse_NOT_SERVING)

	stopped := make(chan struct{})

	go func() {
		server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(timeout):
		server.Stop()
		log.Warn("graceful shutdown timed out, forcing close")
	}
}
