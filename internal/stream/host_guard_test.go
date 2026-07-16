package stream

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHostHeaderGuard pins the DNS-rebinding defense: corsHandler must
// reject any request whose Host header isn't a private/loopback literal
// or localhost. A rebound DNS name (attacker.com → victim LAN IP) drives
// requests with the attacker's Host; without this guard those requests
// reach the frame/control endpoints from the victim's own browser.
func TestHostHeaderGuard(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := corsHandler(inner)

	cases := []struct {
		name       string
		host       string
		wantStatus int
	}{
		{"private IP with port", "192.168.1.5:8080", 200},
		{"loopback", "127.0.0.1:8080", 200},
		{"localhost", "localhost:8080", 200},
		{"10/8", "10.0.0.9:8080", 200},
		{"public DNS name (rebinding)", "attacker.example.com", http.StatusForbidden},
		{"public IP", "8.8.8.8:8080", http.StatusForbidden},
		{"empty host", "", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/info", nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("Host %q: status = %d, want %d", tc.host, rec.Code, tc.wantStatus)
			}
		})
	}
}

// TestFrameEndpointsRequireAuthorizedIP pins that /snapshot and /stream
// only answer the paired client's IP (or loopback). Without an admitted
// client, an arbitrary LAN peer must be refused.
func TestFrameEndpointsRequireAuthorizedIP(t *testing.T) {
	s := NewMJPEGServer("127.0.0.1", 0, nil, nil)

	// No client paired → unauthorized remote IP is refused.
	req := httptest.NewRequest("GET", "/snapshot", nil)
	req.RemoteAddr = "192.168.1.77:5000"
	rec := httptest.NewRecorder()
	s.handleSnapshot(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("unpaired peer got %d from /snapshot, want 403", rec.Code)
	}

	// Loopback (the desktop's own preview) is always allowed — it will
	// 503 for "no frame" rather than 403.
	reqLo := httptest.NewRequest("GET", "/snapshot", nil)
	reqLo.RemoteAddr = "127.0.0.1:5000"
	recLo := httptest.NewRecorder()
	s.handleSnapshot(recLo, reqLo)
	if recLo.Code == http.StatusForbidden {
		t.Errorf("loopback wrongly forbidden from /snapshot")
	}

	// After a client's IP is authorized, that IP is allowed.
	s.frameClientMu.Lock()
	s.frameClientIP = "192.168.1.77"
	s.frameClientMu.Unlock()
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/snapshot", nil)
	req2.RemoteAddr = "192.168.1.77:5001"
	s.handleSnapshot(rec2, req2)
	if rec2.Code == http.StatusForbidden {
		t.Errorf("authorized client IP wrongly forbidden from /snapshot")
	}
}
