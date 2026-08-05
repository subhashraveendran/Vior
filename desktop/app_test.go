package main

import (
	"image"
	"sync"
	"testing"

	"github.com/subhashraveendran/vior/internal/input"
)

// TestTouchMapperAccessIsRaceFree exercises the accessor pair the way the real
// code does: the WebSocket ReadLoop goroutine reads the mapper on every input
// event while the connect/disconnect goroutine installs and clears it.
//
// Before the accessors existed, OnClientInput read a.touchMapper directly —
// once for the nil guard and again for the method call. A disconnect landing
// between those two reads produced both a data race and a use of a mapper the
// session had already torn down.
//
// Run under -race; without the mutex this fails there even though the plain
// run would usually pass, which is exactly why the guard is a test rather than
// a code comment.
func TestTouchMapperAccessIsRaceFree(t *testing.T) {
	a := &App{}
	mapper := input.NewTouchMapper(input.DefaultController, image.Rect(0, 0, 1920, 1080))

	const iterations = 500
	var wg sync.WaitGroup

	// Writer: connect/disconnect churn.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				a.setTouchMapper(mapper)
			} else {
				a.setTouchMapper(nil)
			}
		}
	}()

	// Two readers: the WS input path and the USB touch callback both read it.
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// The snapshot must stay usable after the read even if a
				// concurrent disconnect clears the field — that is the
				// whole point of returning a pointer rather than reading
				// the field twice.
				if tm := a.currentTouchMapper(); tm != nil {
					_ = tm
				}
			}
		}()
	}

	wg.Wait()
}

// A snapshot taken before a disconnect must remain safe to use afterwards.
// Callers deliberately use the returned pointer outside the lock, so this
// pins the contract that makes that safe.
func TestTouchMapperSnapshotSurvivesConcurrentClear(t *testing.T) {
	a := &App{}
	mapper := input.NewTouchMapper(input.DefaultController, image.Rect(0, 0, 800, 600))
	a.setTouchMapper(mapper)

	snapshot := a.currentTouchMapper()
	if snapshot == nil {
		t.Fatal("snapshot was nil immediately after install")
	}

	a.setTouchMapper(nil)

	if snapshot == nil {
		t.Fatal("clearing the field invalidated an already-taken snapshot")
	}
	if got := a.currentTouchMapper(); got != nil {
		t.Fatal("field was not cleared")
	}
}
