package response

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type failingWriter struct {
	header http.Header
	status int
}

func (w *failingWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestJSONAndError(t *testing.T) {
	rr := httptest.NewRecorder()
	JSON(rr, http.StatusCreated, map[string]string{"x": "y"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected content-type json")
	}

	rr = httptest.NewRecorder()
	Error(rr, http.StatusBadRequest, "bad")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400")
	}
	if body := rr.Body.String(); body == "" || body[0] != '{' {
		t.Fatalf("expected json body, got %q", body)
	}

	fw := &failingWriter{}
	JSON(fw, http.StatusOK, map[string]string{"x": "y"})
	if fw.status != http.StatusOK {
		t.Fatalf("expected write header call")
	}
}
