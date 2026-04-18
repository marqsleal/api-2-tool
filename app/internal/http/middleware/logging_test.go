package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestRemoteIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if ip := requestRemoteIP(req); ip != "1.2.3.4" {
		t.Fatalf("unexpected forwarded ip: %s", ip)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "9.9.9.9")
	if ip := requestRemoteIP(req); ip != "9.9.9.9" {
		t.Fatalf("unexpected real ip: %s", ip)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	if ip := requestRemoteIP(req); ip != "10.0.0.1" {
		t.Fatalf("unexpected split host: %s", ip)
	}

	req.RemoteAddr = "invalid"
	if ip := requestRemoteIP(req); ip != "invalid" {
		t.Fatalf("unexpected fallback remote: %s", ip)
	}
}

func TestRequestLogger(t *testing.T) {
	h := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x?q=1", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}
