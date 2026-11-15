package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
)

type clientBucket struct {
	tokens     float64
	lastRefill time.Time
}

// RateLimiter manages in-memory token bucket rate limiting keyed by client IP.
type RateLimiter struct {
	mu           sync.Mutex
	buckets      map[string]*clientBucket
	rate         float64 // tokens per second
	capacity     float64
	cleanupEvery time.Duration
	stopCh       chan struct{}
}

// NewRateLimiter creates a new thread-safe rate limiter.
// limit: number of requests allowed in window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rate := float64(limit) / window.Seconds()
	rl := &RateLimiter{
		buckets:      make(map[string]*clientBucket),
		rate:         rate,
		capacity:     float64(limit),
		cleanupEvery: 5 * time.Minute,
		stopCh:       make(chan struct{}),
	}

	go rl.startCleanupLoop()
	return rl
}

func (rl *RateLimiter) startCleanupLoop() {
	ticker := time.NewTicker(rl.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, b := range rl.buckets {
				if now.Sub(b.lastRefill) > 10*time.Minute {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// Close stops the background cleanup loop.
func (rl *RateLimiter) Close() {
	select {
	case <-rl.stopCh:
	default:
		close(rl.stopCh)
	}
}

// Allow checks if a request from the given IP is within rate limits.
func (rl *RateLimiter) Allow(ip string) (bool, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[ip]
	if !exists {
		rl.buckets[ip] = &clientBucket{
			tokens:     rl.capacity - 1,
			lastRefill: now,
		}
		return true, 0
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0
	}

	// Calculate wait time until 1 token is available
	missing := 1.0 - b.tokens
	retryAfter := int(missing / rl.rate)
	if retryAfter < 1 {
		retryAfter = 1
	}
	return false, retryAfter
}

// LimitHandler wraps an http.Handler with rate limiting for the specified endpoint.
func (rl *RateLimiter) LimitHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		if ip == "" {
			ip = "unknown"
		}

		allowed, retryAfter := rl.Allow(ip)
		if !allowed {
			httperr.TooManyRequests(w, r, retryAfter, "Rate limit exceeded. Please wait before submitting additional requests.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimit returns a middleware function enforcing the specified limit over window duration.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(limit, window)
	return limiter.LimitHandler
}
