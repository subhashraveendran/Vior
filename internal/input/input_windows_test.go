//go:build windows

package input

import "testing"

// captureInput swaps the SendInput seam for the duration of a test and returns
// a pointer to the count of injected events.
func captureInput(t *testing.T) *int {
	t.Helper()
	orig := callSendInput
	var n int
	callSendInput = func([]byte) { n++ }
	t.Cleanup(func() { callSendInput = orig })
	return &n
}

// A chord whose final key is empty must emit nothing at all.
//
// "Shift++" splits to ["Shift", "", ""], so finalKey was empty and none of the
// dispatch cases matched. The old code still pressed and released the
// modifiers around the missing keystroke, sending real modifier events into
// whatever window had focus while the intended key silently vanished.
func TestTypeKeyEmptyFinalKeyEmitsNothing(t *testing.T) {
	c := &winController{}

	cases := []string{
		"Shift++",
		"Ctrl++",
		"Ctrl+Shift++",
		"Ctrl+",
		"Shift+Alt+",
	}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			n := captureInput(t)
			if err := c.TypeKey(key); err != nil {
				t.Fatalf("TypeKey(%q) = %v, want nil", key, err)
			}
			if *n != 0 {
				t.Errorf("TypeKey(%q) emitted %d input events, want 0 — modifiers were pressed around a missing keystroke", key, *n)
			}
		})
	}
}

// The empty-key guard must not swallow legitimate chords.
func TestTypeKeyStillSendsRealChords(t *testing.T) {
	c := &winController{}

	cases := []struct {
		key     string
		wantMin int
		why     string
	}{
		{"a", 2, "plain char: down + up"},
		{"Ctrl+c", 4, "modifier down + key down/up + modifier up"},
		{"Ctrl+Shift+z", 6, "two modifiers + key down/up"},
		{"Enter", 2, "named key: down + up"},
		{"+", 2, "the plus key itself is not a chord"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			n := captureInput(t)
			if err := c.TypeKey(tc.key); err != nil {
				t.Fatalf("TypeKey(%q) = %v, want nil", tc.key, err)
			}
			if *n < tc.wantMin {
				t.Errorf("TypeKey(%q) emitted %d events, want at least %d (%s)", tc.key, *n, tc.wantMin, tc.why)
			}
		})
	}
}

// An empty key was already handled at the top of the function; pin it so the
// new guard does not make that path unreachable-but-untested.
func TestTypeKeyEmptyStringEmitsNothing(t *testing.T) {
	c := &winController{}
	n := captureInput(t)
	if err := c.TypeKey(""); err != nil {
		t.Fatalf("TypeKey(\"\") = %v, want nil", err)
	}
	if *n != 0 {
		t.Errorf("TypeKey(\"\") emitted %d events, want 0", *n)
	}
}
