package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/marqsleal/api-2-tool/internal/config"
)

func TestNewServerAndLifecycle(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	s := NewServer(config.Config{Port: "0"}, h)
	if s.Addr != ":0" {
		t.Fatalf("unexpected addr: %s", s.Addr)
	}
	if s.ReadHeaderTimeout != 3*time.Second {
		t.Fatalf("unexpected read header timeout")
	}

	done := make(chan struct{})
	go func() {
		Start(s)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	Shutdown(s, time.Second)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not stop")
	}

	Shutdown(s, 10*time.Millisecond)
}
