package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxBody    = 4 << 10
	maxEmail   = 254
	maxToolLen = 200
)

// Same shape the page checks before sending; the server is the one that counts.
var emailRE = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)

// The page's <select id="wl-size"> options.
var sizes = map[string]bool{"1-5": true, "6-15": true, "16-40": true, "40+": true}

type signup struct {
	Email string `json:"email"`
	Size  string `json:"size"`
	Tool  string `json:"tool"`
	// Honeypot: a field people never see. Anything in it is a bot.
	Website string `json:"website"`
}

func newHandler(store Store, lim *limiter, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /api/waitlist", func(w http.ResponseWriter, r *http.Request) {
		if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != "application/json" {
			writeErr(w, http.StatusUnsupportedMediaType, "json_only")
			return
		}
		if !lim.allow(clientIP(r), time.Now()) {
			writeErr(w, http.StatusTooManyRequests, "slow_down")
			return
		}

		var in signup
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		if err := dec.Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
		if in.Website != "" {
			// Look like success so the bot has nothing to learn from.
			log.Info("honeypot tripped")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		e, code := validate(in)
		if code != "" {
			writeErr(w, http.StatusBadRequest, code)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := store.Add(ctx, e); err != nil {
			log.Error("store sign-up", "err", err)
			writeErr(w, http.StatusInternalServerError, "store_failed")
			return
		}
		// Addresses stay out of the logs; Loki is readable by more people
		// than the waitlist is.
		log.Info("sign-up stored", "size", e.Size)
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

// validate trims and checks a sign-up, returning an error code for the page.
func validate(in signup) (Entry, string) {
	email := strings.TrimSpace(in.Email)
	if len(email) > maxEmail || !emailRE.MatchString(email) {
		return Entry{}, "invalid_email"
	}
	if !sizes[in.Size] {
		return Entry{}, "invalid_size"
	}
	tool := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(in.Tool))
	if !utf8.ValidString(tool) || utf8.RuneCountInString(tool) > maxToolLen {
		return Entry{}, "invalid_tool"
	}
	return Entry{Email: email, Size: in.Size, Tool: tool}, ""
}

func writeErr(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// clientIP picks the visitor's address. The site sits behind Cloudflare, then
// Traefik: Cloudflare sets CF-Connecting-IP, Traefik sets X-Real-Ip to its
// peer. Only used to key the rate limit, never stored.
func clientIP(r *http.Request) string {
	for _, h := range []string{"CF-Connecting-IP", "X-Real-Ip"} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			return v
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limiter allows n requests per key per window (a fixed window per key).
type limiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	seen   map[string]*bucket
	swept  time.Time
}

type bucket struct {
	start time.Time
	count int
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, seen: map[string]*bucket{}}
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.swept) > l.window {
		for k, b := range l.seen {
			if now.Sub(b.start) > l.window {
				delete(l.seen, k)
			}
		}
		l.swept = now
	}

	b, ok := l.seen[key]
	if !ok || now.Sub(b.start) > l.window {
		l.seen[key] = &bucket{start: now, count: 1}
		return true
	}
	if b.count >= l.n {
		return false
	}
	b.count++
	return true
}
