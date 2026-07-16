package stream

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORSAllowedOrigins pins the policy: same-network + capacitor +
// file echo back the Origin; everything else gets no CORS header so
// the browser drops the response. Wildcard echo was the bug — public
// pages could fetch /info (pair code) + /download/{id} once they had
// the id.
func TestCORSAllowedOrigins(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		allowed bool
	}{
		{"capacitor", "capacitor://localhost", true},
		{"file", "file:///android_asset/index.html", true},
		{"loopback", "http://127.0.0.1:9876", true},
		{"RFC1918 192.168", "http://192.168.1.10:9876", true},
		{"public IPv4", "http://203.0.113.5", false},
		{"public host", "https://attacker.example.com", false},
		{"junk", "not-a-url", false},
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := corsHandler(inner)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/info", nil)
			// Real LAN clients address the server by its private IP; the
			// Host-header (DNS-rebinding) guard requires it. httptest
			// defaults Host to example.com, which the guard rejects.
			req.Host = "192.168.1.5:8080"
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			got := rec.Header().Get("Access-Control-Allow-Origin")
			if tc.allowed {
				if got != tc.origin {
					t.Errorf("origin %q: expected echo, got %q", tc.origin, got)
				}
				if v := rec.Header().Get("Vary"); v != "Origin" {
					t.Errorf("origin %q: Vary header missing, got %q", tc.origin, v)
				}
			} else if got != "" {
				t.Errorf("origin %q: expected no CORS header, got %q", tc.origin, got)
			}
		})
	}
}

// TestCORSNoOriginPassThrough: native clients (no Origin) are unaffected
// — handler still runs and no CORS headers leak.
func TestCORSNoOriginPassThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) })
	req := httptest.NewRequest("GET", "/info", nil)
	req.Host = "192.168.1.5:8080" // private Host so the DNS-rebinding guard admits it
	rec := httptest.NewRecorder()
	corsHandler(inner).ServeHTTP(rec, req)
	if !called {
		t.Fatal("inner handler not invoked for native client")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("CORS header set for native client request: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
