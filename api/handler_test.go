package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu      sync.Mutex
	entries []Entry
	addErr  error
	pingErr error
}

func (f *fakeStore) Add(_ context.Context, e Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return f.addErr
	}
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }

func newTestHandler(s Store, lim *limiter) http.Handler {
	if lim == nil {
		lim = newLimiter(100, time.Minute)
	}
	return newHandler(s, lim, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func post(h http.Handler, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSignup(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
		code   string
		stored *Entry
	}{
		{"valid", `{"email":" Ada@Studio.com ","size":"6-15","tool":" Harvest "}`, 204, "",
			&Entry{Email: "Ada@Studio.com", Size: "6-15", Tool: "Harvest"}},
		{"no tool", `{"email":"a@b.co","size":"40+"}`, 204, "", &Entry{Email: "a@b.co", Size: "40+"}},
		{"control chars stripped", `{"email":"a@b.co","size":"1-5","tool":"Sheets\u0000\n"}`, 204, "",
			&Entry{Email: "a@b.co", Size: "1-5", Tool: "Sheets"}},
		{"bad email", `{"email":"nope","size":"1-5"}`, 400, "invalid_email", nil},
		{"email too long", `{"email":"` + strings.Repeat("a", 250) + `@b.co","size":"1-5"}`, 400, "invalid_email", nil},
		{"bad size", `{"email":"a@b.co","size":"1000"}`, 400, "invalid_size", nil},
		{"tool too long", `{"email":"a@b.co","size":"1-5","tool":"` + strings.Repeat("x", 201) + `"}`, 400, "invalid_tool", nil},
		{"not json", `email=a@b.co`, 400, "bad_request", nil},
		{"too big", `{"email":"a@b.co","size":"1-5","tool":"` + strings.Repeat("x", 5000) + `"}`, 400, "bad_request", nil},
		{"honeypot", `{"email":"a@b.co","size":"1-5","website":"http://spam"}`, 204, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeStore{}
			rec := post(newTestHandler(s, nil), tc.body, nil)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d (body %s)", rec.Code, tc.status, rec.Body)
			}
			if tc.code != "" && !strings.Contains(rec.Body.String(), `"`+tc.code+`"`) {
				t.Fatalf("body %s, want code %s", rec.Body, tc.code)
			}
			switch {
			case tc.stored == nil && len(s.entries) != 0:
				t.Fatalf("stored %+v, want nothing", s.entries)
			case tc.stored != nil && (len(s.entries) != 1 || s.entries[0] != *tc.stored):
				t.Fatalf("stored %+v, want %+v", s.entries, *tc.stored)
			}
		})
	}
}

func TestSignupRejectsNonJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/waitlist", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	newTestHandler(&fakeStore{}, nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d, want 415", rec.Code)
	}
}

func TestSignupStoreFailure(t *testing.T) {
	rec := post(newTestHandler(&fakeStore{addErr: errors.New("db down")}, nil), `{"email":"a@b.co","size":"1-5"}`, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
}

func TestRateLimitPerClient(t *testing.T) {
	h := newTestHandler(&fakeStore{}, newLimiter(2, time.Minute))
	body := `{"email":"a@b.co","size":"1-5"}`
	a := map[string]string{"CF-Connecting-IP": "203.0.113.1"}
	b := map[string]string{"CF-Connecting-IP": "203.0.113.2"}
	for i, want := range []int{204, 204, 429} {
		if got := post(h, body, a).Code; got != want {
			t.Fatalf("client a request %d: status %d, want %d", i+1, got, want)
		}
	}
	if got := post(h, body, b).Code; got != 204 {
		t.Fatalf("client b: status %d, want 204", got)
	}
}

func TestLimiterWindowResets(t *testing.T) {
	l := newLimiter(1, time.Minute)
	now := time.Now()
	if !l.allow("k", now) || l.allow("k", now.Add(time.Second)) {
		t.Fatal("first request should pass, second should not")
	}
	if !l.allow("k", now.Add(61*time.Second)) {
		t.Fatal("request after the window should pass")
	}
}

func TestHealth(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{nil, 200}, {errors.New("down"), 503}} {
		rec := httptest.NewRecorder()
		newTestHandler(&fakeStore{pingErr: tc.err}, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
		if rec.Code != tc.want {
			t.Fatalf("ping err %v: status %d, want %d", tc.err, rec.Code, tc.want)
		}
	}
}

func TestOnlyPostOnWaitlist(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(&fakeStore{}, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/waitlist", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rec.Code)
	}
}
