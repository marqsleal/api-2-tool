package app

import (
	"context"
	"errors"
	"log"
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
	log.Printf("server running on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func Shutdown(server *http.Server, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown err: %v", err)
	}
}
