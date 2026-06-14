package stream

import (
	"fmt"
	"testing"
)

// TestPairGlobalRateLimit pins the new server-wide ceiling. A
// distributed brute force across many source IPs slipped past the
// per-IP throttle entirely; the global bucket catches that pattern.
// maxGlobalPairAttempts wrong codes in a minute (from any source) is
// the cutoff.
func TestPairGlobalRateLimit(t *testing.T) {
	// Reset state so prior tests can't bleed in.
	pairAttemptsMu.Lock()
	globalPairAttempts.times = globalPairAttempts.times[:0]
	for k := range pairAttempts {
		delete(pairAttempts, k)
	}
	pairAttemptsMu.Unlock()

	// Each "attacker" only burns 1 attempt, well below the per-IP cap.
	// The global bucket is what eventually says "over".
	for i := 0; i < maxGlobalPairAttempts; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		if over := recordPairAttempt(ip); over {
			t.Fatalf("global ceiling tripped early at attempt %d (max=%d)", i+1, maxGlobalPairAttempts)
		}
	}
	// The next attempt from a fresh, never-seen IP must be rejected
	// by the global bucket even though that IP's per-IP bucket is empty.
	if over := recordPairAttempt("10.0.0.250"); !over {
		t.Fatalf("global ceiling should refuse the %dth attempt across all IPs", maxGlobalPairAttempts+1)
	}
}

// TestPairEmptyIPStillCountsGlobally — recordPairAttempt("") previously
// returned false immediately, letting an attacker who could strip the
// IP header bypass the throttle entirely. Empty IP must still consume
// the global bucket.
func TestPairEmptyIPStillCountsGlobally(t *testing.T) {
	pairAttemptsMu.Lock()
	globalPairAttempts.times = globalPairAttempts.times[:0]
	pairAttemptsMu.Unlock()

	for i := 0; i < maxGlobalPairAttempts; i++ {
		if over := recordPairAttempt(""); over {
			t.Fatalf("empty-IP attempt %d already over (max=%d)", i+1, maxGlobalPairAttempts)
		}
	}
	if over := recordPairAttempt(""); !over {
		t.Fatal("empty-IP attempt past global ceiling should be over")
	}
}
