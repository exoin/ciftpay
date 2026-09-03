// Package httpx holds the chi middleware stack and the JSON/error helpers
// that implement the error envelope from ADR-0006.
package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// Error is the envelope body for non-2xx responses.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type envelope struct {
	Error Error `json:"error"`
}

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Fail writes an error envelope.
func Fail(w http.ResponseWriter, status int, code, message string, details ...map[string]any) {
	e := Error{Code: code, Message: message}
	if len(details) > 0 {
		e.Details = details[0]
	}
	JSON(w, status, envelope{Error: e})
}

// Decode reads a JSON body of at most 1 MiB.
func Decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// -------------------------------------------------------------- middleware

// RequestID adds/propagates X-Request-ID and puts it on the context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

// Logger logs one line per request with slog, never bodies.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			lvl := slog.LevelInfo
			if ww.Status() >= 500 {
				lvl = slog.LevelError
			}
			log.Log(r.Context(), lvl, "http",
				"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"bytes", ww.BytesWritten(), "ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()), "ip", ClientIP(r))
		})
	}
}

// Recover turns panics into 500 envelopes.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "err", rec, "path", r.URL.Path, "request_id", RequestIDFrom(r.Context()))
					Fail(w, http.StatusInternalServerError, "internal", "Something went wrong on our side")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS allows the PWA origin(s) with credentials.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Request-ID")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				h.Set("Vary", "Origin")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets conservative defaults for an API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// RateLimit is a fixed-window in-memory limiter keyed by the given function.
// Good enough for OTP and webhook endpoints on a single api instance; swap
// for a Postgres/Redis-backed limiter when the api scales out.
func RateLimit(limit int, window time.Duration, key func(*http.Request) string) func(http.Handler) http.Handler {
	type bucket struct {
		n     int
		reset time.Time
	}
	var mu sync.Mutex
	buckets := map[string]*bucket{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := key(r)
			now := time.Now()
			mu.Lock()
			b := buckets[k]
			if b == nil || now.After(b.reset) {
				b = &bucket{reset: now.Add(window)}
				buckets[k] = b
				if len(buckets) > 50000 {
					for kk, bb := range buckets {
						if now.After(bb.reset) {
							delete(buckets, kk)
						}
					}
				}
			}
			b.n++
			over := b.n > limit
			retry := time.Until(b.reset)
			mu.Unlock()
			if over {
				w.Header().Set("Retry-After", strings.TrimSuffix((retry/time.Second*time.Second).String(), "0s"))
				Fail(w, http.StatusTooManyRequests, "rate_limited", "Too many requests, slow down")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPAllowlist rejects requests whose client IP is outside the list. An empty
// list disables the check (local development).
func IPAllowlist(cidrs []string) func(http.Handler) http.Handler {
	var nets []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			c += "/32"
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	return func(next http.Handler) http.Handler {
		if len(nets) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := net.ParseIP(ClientIP(r))
			for _, n := range nets {
				if ip != nil && n.Contains(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}
			Fail(w, http.StatusForbidden, "forbidden", "Source address not allowed")
		})
	}
}

// ClientIP prefers X-Forwarded-For (first hop) then RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ------------------------------------------------------------ context keys

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxPrincipal
)

// Principal is the authenticated caller placed on the context by the org
// package's session middleware.
type Principal struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	SessionID uuid.UUID
	CSRF      string
	Locale    string
}

// RequestIDFrom returns the request id or "".
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestID).(string)
	return v
}

// WithPrincipal stores p on ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxPrincipal, p)
}

// PrincipalFrom returns the principal and whether one is present.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxPrincipal).(Principal)
	return p, ok
}
