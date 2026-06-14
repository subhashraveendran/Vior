package stream

import (
	"testing"
)

// TestPairRateLimit asserts the per-IP brute-force throttle on the
// pair-code handshake: maxPairAttempts wrong codes / minute / IP fire
// pair_mismatch, after that the IP is over the limit.
func TestPairRateLimit(t *testing.T) {
	// Drain any state from previous tests — including the new global
	// bucket, which is shared across IPs and would otherwise carry
	// counts from TestPairGlobalRateLimit into here.
	clearPairAttempts("198.51.100.7")
	pairAttemptsMu.Lock()
	globalPairAttempts.times = globalPairAttempts.times[:0]
	pairAttemptsMu.Unlock()

	for i := 1; i <= maxPairAttempts; i++ {
		if over := recordPairAttempt("198.51.100.7"); over {
			t.Fatalf("attempt %d was already over the limit (max=%d)", i, maxPairAttempts)
		}
	}
	if over := recordPairAttempt("198.51.100.7"); !over {
		t.Fatalf("attempt %d should be over the limit", maxPairAttempts+1)
	}

	// A different IP must not inherit the ban.
	if over := recordPairAttempt("198.51.100.8"); over {
		t.Fatalf("unrelated IP must start fresh")
	}

	// clearPairAttempts resets the bucket so a user who finally types the
	// right code is not penalized.
	clearPairAttempts("198.51.100.7")
	if over := recordPairAttempt("198.51.100.7"); over {
		t.Fatalf("after clear, first attempt must not be over")
	}
}

func TestRemoteIP(t *testing.T) {
	cases := map[string]string{
		"192.168.1.10:54321": "192.168.1.10",
		"[::1]:54321":        "::1",
		"":                   "",
	}
	for in, want := range cases {
		if got := remoteIP(in); got != want {
			t.Errorf("remoteIP(%q) = %q want %q", in, got, want)
		}
	}
}
