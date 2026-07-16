package stream

import (
	"sync"
	"testing"
	"time"
)

// TestRemoveClientAfterStopNoPanic is a regression test for the shutdown
// double-close panic: Stop() closes every client channel, then a
// handleStream defer fires removeClient() on the same channel. The old
// removeClient closed unconditionally → close of a closed channel →
// panic. removeClient must be a no-op once Stop has removed the client.
func TestRemoveClientAfterStopNoPanic(t *testing.T) {
	fc := make(chan []byte)
	s := NewMJPEGServer("127.0.0.1", 0, fc, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ch, err := s.addClient()
	if err != nil {
		t.Fatalf("addClient: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Would panic on a double close before the fix.
	s.removeClient(ch)
}

// TestSetFrameChChurnNoRace hammers SetFrameCh, addClient/removeClient
// concurrently to shake out data races on the distributor lifecycle
// fields. Run with -race.
func TestSetFrameChChurnNoRace(t *testing.T) {
	s := NewMJPEGServer("127.0.0.1", 0, nil, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	var wg sync.WaitGroup
	// Broadcast stop via a CLOSED channel — a time.After channel only
	// delivers its tick to ONE receiver, so sharing it across goroutines
	// leaves the others spinning on default forever.
	stop := make(chan struct{})
	time.AfterFunc(300*time.Millisecond, func() { close(stop) })

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				ch := make(chan []byte)
				s.SetFrameCh(ch)
				close(ch)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				if ch, err := s.addClient(); err == nil {
					s.removeClient(ch)
				}
			}
		}
	}()

	wg.Wait()
}
