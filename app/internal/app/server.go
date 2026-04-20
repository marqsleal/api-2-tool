package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/marqsleal/api-2-tool/internal/config"
)

func NewServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
	}
}

func Start(server *http.Server) error {
	slog.Info("server_running", "component", "app.server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func Shutdown(server *http.Server, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("server_shutdown_error", "component", "app.server", "error", err)
	}
}
