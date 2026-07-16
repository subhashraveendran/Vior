package session

import (
	"strings"
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

// TestConfigureRejectsBadDimensions pins the OOM-guard branches of
// Configure. These validations run before any virtual-display or
// capture platform call, so they are safe to test headless. An
// unbounded or non-positive width/height from an untrusted client
// would otherwise multiply into a giant RGBA allocation.
func TestConfigureRejectsBadDimensions(t *testing.T) {
	cases := []struct {
		name          string
		w, h          int
		wantErrSubstr string
	}{
		{"zero width", 0, 800, "invalid client dimensions"},
		{"zero height", 1280, 0, "invalid client dimensions"},
		{"negative width", -1, 800, "invalid client dimensions"},
		{"negative height", 1280, -5, "invalid client dimensions"},
		{"width over max", maxClientDimension + 1, 800, "exceed max"},
		{"height over max", 1280, maxClientDimension + 1, "exceed max"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Configure(&protocol.HelloMessage{
				Width:  tc.w,
				Height: tc.h,
				DPR:    2.0,
				Mode:   "extend",
			})
			if err == nil {
				t.Fatalf("Configure(%dx%d) accepted bad dimensions", tc.w, tc.h)
			}
			if !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Errorf("Configure(%dx%d) err = %q, want substring %q", tc.w, tc.h, err.Error(), tc.wantErrSubstr)
			}
		})
	}
}

// TestConfigureAcceptsBoundaryDimensions confirms the exact max is
// allowed (off-by-one guard) — but since the accept path touches the
// capture/virtual platform layer, we only assert it does NOT fail on
// the dimension check itself. A platform error is tolerated/skipped.
func TestConfigureBoundaryDimensionNotRejectedForSize(t *testing.T) {
	_, err := Configure(&protocol.HelloMessage{
		Width:  maxClientDimension,
		Height: maxClientDimension,
		DPR:    1.0,
		Intent: "remote", // skip path — avoids creating a real virtual display
	})
	// remote intent takes the skip branch, which lists displays; on a
	// headless box that may error, but it must NOT be the dimension
	// rejection.
	if err != nil && (strings.Contains(err.Error(), "invalid client dimensions") || strings.Contains(err.Error(), "exceed max")) {
		t.Fatalf("boundary dimensions wrongly rejected by size check: %v", err)
	}
}
