package middleware

import (
	"log"
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

		log.Printf(
			"request started method=%s path=%s raw_query=%q remote_ip=%s user_agent=%q",
			r.Method,
			r.URL.Path,
			r.URL.RawQuery,
			remoteIP,
			r.UserAgent(),
		)

		loggedWriter := &loggingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(loggedWriter, r)

		duration := time.Since(start)
		log.Printf(
			"request finished method=%s path=%s status=%d bytes=%d duration=%s remote_ip=%s",
			r.Method,
			r.URL.Path,
			loggedWriter.status,
			loggedWriter.bytes,
			duration.Round(time.Millisecond),
			remoteIP,
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
