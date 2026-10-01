package stream

import (
	"net"
	"testing"
)

// TestStartPortInUseRollsBack covers the Listen-failure path in Start.
//
// Before the rollback was added, a port-in-use error left `running` set, so
// IsRunning() reported true for a server that never bound a socket: the
// desktop then refused every later StartServer with "already running" until
// the app was relaunched, and the frame distributor goroutine leaked. The
// same instance must be startable again once a port is free.
func TestStartPortInUseRollsBack(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	fc := make(chan []byte)
	s := NewMJPEGServer("127.0.0.1", port, fc, nil)
	if err := s.Start(); err == nil {
		t.Fatalf("Start on occupied port %d succeeded", port)
	}
	if s.IsRunning() {
		t.Fatalf("IsRunning() = true after failed Start")
	}

	// The distributor started before Listen must have been torn down so a
	// retry can install a fresh one.
	s.distMu.Lock()
	running := s.distRunning
	s.distMu.Unlock()
	if running {
		t.Fatalf("frame distributor still running after failed Start")
	}

	// Retry on a free port with the same instance.
	s.mu.Lock()
	s.port = 0
	s.mu.Unlock()
	if err := s.Start(); err != nil {
		t.Fatalf("retry Start: %v", err)
	}
	defer s.Stop()
	if !s.IsRunning() {
		t.Fatalf("IsRunning() = false after successful retry")
	}
}
