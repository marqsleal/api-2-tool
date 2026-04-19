package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/marqsleal/api-2-tool/internal/app"
	"github.com/marqsleal/api-2-tool/internal/config"
	"github.com/marqsleal/api-2-tool/internal/http/handler"
	"github.com/marqsleal/api-2-tool/internal/http/router"
	"github.com/marqsleal/api-2-tool/internal/repository"
	"github.com/marqsleal/api-2-tool/internal/service"
)

func main() {
	cfg := config.Load()

	healthService := service.NewHealthService()
	healthHandler := handler.NewHealthHandler(healthService)

	toolDefinitionRepository, err := repository.NewSQLiteToolDefinitionRepository(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("failed to initialize sqlite repository: %v", err)
	}
	defer func() {
		if err := toolDefinitionRepository.Close(); err != nil {
			log.Printf("failed to close sqlite repository: %v", err)
		}
	}()

	toolDefinitionService := service.NewToolDefinitionService(toolDefinitionRepository)
	toolExecutorService := service.NewToolExecutorService(toolDefinitionService)
	toolHandler := handler.NewToolHandler(toolDefinitionService, toolExecutorService)
	openAPISpecHandler := handler.NewOpenAPISpecHandler(cfg.OpenAPISpecPath)
	swaggerUIHandler := handler.NewSwaggerUIHandler("/swagger/doc.yaml")

	httpRouter := router.New(healthHandler, toolHandler, swaggerUIHandler, openAPISpecHandler)
	server := app.NewServer(cfg, httpRouter)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- app.Start(server)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatalf("server failed: %v", err)
		}
		return
	case <-stop:
		log.Println("shutdown initiated")
	}

	app.Shutdown(server, cfg.ShutdownTimeout)
	if err := <-serverErr; err != nil {
		log.Printf("server stopped with error: %v", err)
	}
	log.Println("server stopped")
}
