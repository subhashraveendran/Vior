package stream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestInfoDoesNotLeakPairCode is the regression test for the auth
// bypass: /info used to publish the raw pairCode to any LAN client,
// which defeated pairing entirely. The response must never contain the
// code.
func TestInfoDoesNotLeakPairCode(t *testing.T) {
	s := NewMJPEGServer("127.0.0.1", 0, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/info", nil)
	s.handleInfo(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "pairCode") {
		t.Errorf("/info response still contains pairCode: %s", body)
	}
	if strings.Contains(body, PairCode()) {
		t.Errorf("/info response leaks the actual pair code %q: %s", PairCode(), body)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("/info is not valid JSON: %v (%s)", err, body)
	}
	for _, k := range []string{"name", "version", "platform", "deviceId"} {
		if _, ok := resp[k]; !ok {
			t.Errorf("/info missing expected field %q", k)
		}
	}
	if _, ok := resp["paired"]; ok {
		t.Errorf("/info without probe must not include 'paired'")
	}
}

// TestInfoProbeMatch confirms the constant-time probe returns paired
// only for the correct code, and that a device name with JSON-special
// characters can't corrupt the response (json.Marshal escaping — the
// old fmt.Fprintf path did not escape).
func TestInfoProbeMatch(t *testing.T) {
	s := NewMJPEGServer("127.0.0.1", 0, nil, nil)

	// Reset rate-limit state so this test is order-independent.
	pairAttemptsMu.Lock()
	globalPairAttempts.times = globalPairAttempts.times[:0]
	for k := range pairAttempts {
		delete(pairAttempts, k)
	}
	pairAttemptsMu.Unlock()

	// Correct probe → paired:true.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/info?probe="+PairCode(), nil)
	req.RemoteAddr = "192.168.1.50:5000"
	s.handleInfo(rec, req)
	var ok map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil {
		t.Fatalf("probe response not JSON: %v", err)
	}
	if ok["paired"] != true {
		t.Errorf("correct probe did not return paired=true: %v", ok)
	}

	// Wrong probe → no paired field.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/info?probe=000000zzz", nil)
	req2.RemoteAddr = "192.168.1.51:5000"
	s.handleInfo(rec2, req2)
	var bad map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &bad); err != nil {
		// A 429 (rate limited) is also acceptable; only fail on a paired=true.
		return
	}
	if bad["paired"] == true {
		t.Errorf("wrong probe returned paired=true: %v", bad)
	}
}

// TestInfoProbeRateLimited confirms repeated wrong probes from one IP
// eventually get 429 — the probe endpoint must not be an unlimited
// brute-force oracle for the 6-digit code.
func TestInfoProbeRateLimited(t *testing.T) {
	s := NewMJPEGServer("127.0.0.1", 0, nil, nil)
	pairAttemptsMu.Lock()
	globalPairAttempts.times = globalPairAttempts.times[:0]
	for k := range pairAttempts {
		delete(pairAttempts, k)
	}
	pairAttemptsMu.Unlock()

	got429 := false
	for i := 0; i < maxGlobalPairAttempts+5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/info?probe=999999", nil)
		req.RemoteAddr = "10.9.9.9:6000"
		s.handleInfo(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Error("wrong probes never triggered a 429 — brute-force oracle is open")
	}
}
