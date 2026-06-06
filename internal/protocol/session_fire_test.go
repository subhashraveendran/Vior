package protocol

import (
	"sync"
	"testing"
)

// TestFireDisconnectRunsOnce verifies the sync.Once guarding
// OnClientDisconnect — even from many goroutines, the cleanup callback
// must execute exactly once. Regression test for the Bye-then-defer
// double-tear-down of the macOS virtual display.
func TestFireDisconnectRunsOnce(t *testing.T) {
	s := &Session{}
	var count int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.FireDisconnect(func() {
				mu.Lock()
				count++
				mu.Unlock()
			})
		}()
	}
	wg.Wait()
	if count != 1 {
		t.Fatalf("FireDisconnect ran %d times, want 1", count)
	}
}
