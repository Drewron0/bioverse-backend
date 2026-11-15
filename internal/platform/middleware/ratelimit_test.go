package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(3, 1*time.Second)
	defer limiter.Close()

	handler := limiter.LimitHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 3 requests from same IP should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected status 200, got %d", i+1, rec.Code)
		}
	}

	// 4th request from same IP should be blocked with 429
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request 4 expected status 429, got %d", rec.Code)
	}

	// Request from a different IP should succeed
	reqOther := httptest.NewRequest("GET", "/test", nil)
	reqOther.RemoteAddr = "192.168.1.2:12345"
	recOther := httptest.NewRecorder()
	handler.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusOK {
		t.Fatalf("request from other IP expected status 200, got %d", recOther.Code)
	}
}
