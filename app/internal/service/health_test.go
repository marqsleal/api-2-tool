package service

import "testing"

func TestHealthServiceStatus(t *testing.T) {
	svc := NewHealthService()
	status := svc.Status()
	if status.Status != "ok" {
		t.Fatalf("expected ok, got %s", status.Status)
	}
}
