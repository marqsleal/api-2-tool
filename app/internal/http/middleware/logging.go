package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(body []byte) (int, error) {
	size, err := w.ResponseWriter.Write(body)
	w.bytes += size
	return size, err
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		remoteIP := requestRemoteIP(r)
		requestID := RequestIDFromContext(r.Context())

		slog.InfoContext(
			r.Context(),
			"request_started",
			"component", "http.middleware",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"raw_query", r.URL.RawQuery,
			"remote_ip", remoteIP,
			"user_agent", r.UserAgent(),
		)

		loggedWriter := &loggingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(loggedWriter, r)

		duration := time.Since(start)
		slog.InfoContext(
			r.Context(),
			"request_finished",
			"component", "http.middleware",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", loggedWriter.status,
			"bytes", loggedWriter.bytes,
			"duration_ms", duration.Milliseconds(),
			"remote_ip", remoteIP,
		)
	})
}

func requestRemoteIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}

	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}
