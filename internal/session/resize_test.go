package session

import (
	"testing"

	"github.com/subhashraveendran/vior/internal/protocol"
)

func TestResizeHelloCarriesIntentAndSwitchesMode(t *testing.T) {
	prev := &protocol.HelloMessage{
		Name: "Pixel", Width: 1080, Height: 2400, DPR: 2.5, Mode: "mirror",
		Intent: "remote", SkipDisplay: true, DeviceID: "mob-1", PairCode: "123456",
	}

	rotated := ResizeHello(prev, &protocol.ResizeMessage{Width: 2400, Height: 1080, DPR: 2.5})
	if rotated.Width != 2400 || rotated.Height != 1080 {
		t.Fatalf("dimensions not applied: %dx%d", rotated.Width, rotated.Height)
	}
	if rotated.Mode != "mirror" {
		t.Fatalf("mode = %q, want the session's mirror kept", rotated.Mode)
	}
	if rotated.Intent != "remote" || !rotated.SkipDisplay || rotated.DeviceID != "mob-1" {
		t.Fatalf("session identity/intent dropped: %+v", rotated)
	}
	if prev.Width != 1080 {
		t.Fatalf("ResizeHello mutated the previous hello")
	}

	switched := ResizeHello(prev, &protocol.ResizeMessage{Width: 1080, Height: 2400, DPR: 2.5, Mode: "extend"})
	if switched.Mode != "extend" {
		t.Fatalf("mode = %q, want extend from the resize", switched.Mode)
	}

	legacy := ResizeHello(nil, &protocol.ResizeMessage{Width: 800, Height: 600, DPR: 1})
	if legacy.Mode != "extend" {
		t.Fatalf("mode = %q, want extend default with no prior hello", legacy.Mode)
	}
}
