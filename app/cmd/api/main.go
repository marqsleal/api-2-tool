package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marqsleal/api-2-tool/internal/app"
	"github.com/marqsleal/api-2-tool/internal/config"
	"github.com/marqsleal/api-2-tool/internal/http/handler"
	"github.com/marqsleal/api-2-tool/internal/http/router"
	"github.com/marqsleal/api-2-tool/internal/logging"
	"github.com/marqsleal/api-2-tool/internal/repository"
	"github.com/marqsleal/api-2-tool/internal/service"
)

func main() {
	cfg := config.Load()
	slog.SetDefault(logging.New(cfg.LogLevel, cfg.HumanLog, cfg.LogIncludeSource))

	healthService := service.NewHealthService()
	healthHandler := handler.NewHealthHandler(healthService)

	toolDefinitionRepository, err := repository.NewSQLiteToolDefinitionRepository(cfg.SQLitePath)
	if err != nil {
		slog.Error("failed_to_initialize_sqlite_repository", "component", "main", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := toolDefinitionRepository.Close(); err != nil {
			slog.Error("failed_to_close_sqlite_repository", "component", "main", "error", err)
		}
	}()

	circuitBreakerRepository, err := repository.NewSQLiteCircuitBreakerRepository(cfg.SQLitePath)
	if err != nil {
		slog.Error("failed_to_initialize_circuit_breaker_repository", "component", "main", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := circuitBreakerRepository.Close(); err != nil {
			slog.Error("failed_to_close_circuit_breaker_repository", "component", "main", "error", err)
		}
	}()

	toolJobRepository, err := repository.NewSQLiteToolJobRepository(cfg.SQLitePath)
	if err != nil {
		slog.Error("failed_to_initialize_job_repository", "component", "main", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := toolJobRepository.Close(); err != nil {
			slog.Error("failed_to_close_job_repository", "component", "main", "error", err)
		}
	}()

	toolDefinitionService := service.NewToolDefinitionService(toolDefinitionRepository)
	circuitBreakerService := service.NewCircuitBreakerService(circuitBreakerRepository, 5, 30*time.Second, 2)
	toolExecutorService := service.NewToolExecutorServiceWithOptions(toolDefinitionService, &circuitBreakerService, nil)
	toolJobService := service.NewToolJobService(toolJobRepository, toolExecutorService)
	toolJobService.SetRetention(cfg.JobRetention)
	toolHandler := handler.NewToolHandler(toolDefinitionService, toolExecutorService, &toolJobService)
	openAPISpecHandler := handler.NewOpenAPISpecHandler(cfg.OpenAPISpecPath)
	swaggerUIHandler := handler.NewSwaggerUIHandler("/swagger/doc.yaml")

	httpRouter := router.New(healthHandler, toolHandler, swaggerUIHandler, openAPISpecHandler)
	server := app.NewServer(cfg, httpRouter)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	serverErr := make(chan error, 1)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	if err := toolJobService.StartWorkers(workerCtx, cfg.JobWorkerCount); err != nil {
		slog.Error("failed_to_start_job_workers", "component", "main", "error", err)
		os.Exit(1)
	}
	go func() {
		serverErr <- app.Start(server)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			slog.Error("server_failed", "component", "main", "error", err)
			os.Exit(1)
		}
		return
	case <-stop:
		slog.Info("shutdown_initiated", "component", "main")
	}

	workerCancel()
	app.Shutdown(server, cfg.ShutdownTimeout)
	if err := <-serverErr; err != nil {
		slog.Error("server_stopped_with_error", "component", "main", "error", err)
	}
	slog.Info("server_stopped", "component", "main")
}
