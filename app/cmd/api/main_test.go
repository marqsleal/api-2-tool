package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMainBootAndShutdownSignal(t *testing.T) {
	t.Setenv("APP_PORT", "0")
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "tools.db"))

	specPath := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(specPath, []byte("openapi: 3.0.3\n"), 0o600); err != nil {
		t.Fatalf("write spec file: %v", err)
	}
	t.Setenv("OPENAPI_SPEC_PATH", specPath)

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send signal: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("main did not shutdown after signal")
	}
}
