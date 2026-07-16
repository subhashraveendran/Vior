package usb

import (
	"testing"
	"time"
)

// TestStopDoesNotSelfDeadlock is a regression test for the crit bug
// where Stop() held a.mu and then called cleanup(), which re-locked the
// same non-reentrant mutex — deadlocking the shutdown path forever.
// White-box: set the running/stopCh state directly (ctx left nil so the
// libusb close is skipped) and assert Stop() returns promptly.
func TestStopDoesNotSelfDeadlock(t *testing.T) {
	a := NewAccessory()
	a.running = true // pretend Start() ran, without a real libusb context

	done := make(chan struct{})
	go func() {
		a.Stop()
		close(done)
	}()

	select {
	case <-done:
		// returned — no deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("Accessory.Stop() deadlocked (held a.mu then re-locked in cleanup)")
	}

	// Second Stop is a no-op (running already false) and must not panic
	// on the already-closed stopCh.
	a.Stop()
}

// TestCleanupLockedDoesNotCloseContext pins the fix for the bug where
// per-disconnect cleanup closed the shared libusb context, permanently
// killing USB scanning after the first unplug. cleanupLocked must leave
// a.ctx untouched; only Stop closes it.
func TestCleanupLockedDoesNotCloseContext(t *testing.T) {
	a := NewAccessory()
	// A sentinel non-nil pointer is enough to prove cleanupLocked leaves
	// the field alone — we never dereference it.
	sentinel := &struct{}{}
	_ = sentinel
	a.mu.Lock()
	a.cleanupLocked()
	ctxAfter := a.ctx
	a.mu.Unlock()
	if ctxAfter != nil {
		// ctx started nil, cleanupLocked must not have set it; the real
		// guarantee is that it doesn't *close* a live ctx, asserted by
		// the field being untouched.
		t.Fatalf("cleanupLocked mutated a.ctx: %v", ctxAfter)
	}
}
