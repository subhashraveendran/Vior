package handshake

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestJSClientMatchesVectors runs the JavaScript secure-channel client's
// vector suite (mobile-cap/tools/secure-channel-test.mjs), which replays
// testdata/handshake_vectors.json through clients/secure/vior-secure.js.
//
// The Go and JS implementations share no code, only the committed vectors;
// this test makes a divergence fail `go test ./...` on any machine that has
// node available, instead of only surfacing when a phone connects. Skipped
// (not failed) when node is not installed, e.g. in a Go-only CI stage.
func TestJSClientMatchesVectors(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping JS interop suite")
	}

	// go test runs with the package directory as CWD.
	script := filepath.Join("..", "..", "mobile-cap", "tools", "secure-channel-test.mjs")
	cmd := exec.Command(node, script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("JS secure-channel suite failed: %v\n%s", err, out)
	}
	t.Logf("JS secure-channel suite:\n%s", tail(out, 400))
}

// tail returns the last n bytes of b, for terse passing-run logs.
func tail(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}
