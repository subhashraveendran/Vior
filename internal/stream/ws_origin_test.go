package stream

import (
	"net/http"
	"testing"
)

// TestCheckWSOrigin pins the WS upgrade gate: native clients (no
// Origin), loopback, link-local and RFC1918 hosts are admitted; public
// origins and unparseable junk are rejected. Without this gate any web
// page on the open internet could ride the user's pre-validated
// pair-code session to talk to the local Vior WS server.
func TestCheckWSOrigin(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"empty (native WS client)", "", true},
		{"localhost", "http://localhost", true},
		{"localhost with port", "http://localhost:34567", true},
		{"loopback v4", "http://127.0.0.1:34567", true},
		{"loopback v6", "http://[::1]:34567", true},
		{"link-local v4", "http://169.254.10.1", true},
		{"RFC1918 10/8", "http://10.0.0.5:8080", true},
		{"RFC1918 172.16/12", "http://172.20.0.1", true},
		{"RFC1918 192.168/16", "http://192.168.1.42", true},
		{"public IPv4", "http://8.8.8.8", false},
		{"public hostname", "https://evil.example.com", false},
		{"unparseable origin", "http://%zz", false},
		{"junk", "not-a-url", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if got := checkWSOrigin(req); got != tc.want {
				t.Errorf("checkWSOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}
