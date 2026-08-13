package protocol

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/subhashraveendran/vior/internal/securechan"
)

// TestSendFrameCleartextReturnsErrNotSecure: frames must never ride an
// unsealed WebSocket. A cleartext session gets the sentinel so the stream
// layer leaves its frames on the MJPEG path.
func TestSendFrameCleartextReturnsErrNotSecure(t *testing.T) {
	s := &Session{}
	if err := s.SendFrame([]byte{0xFF, 0xD8}); !errors.Is(err, ErrNotSecure) {
		t.Fatalf("SendFrame on cleartext session = %v, want ErrNotSecure", err)
	}
}

// TestSendFrameOversizeReturnsErrFrameTooLarge: a frame whose sealed size
// would blow the peer's secure read limit must be refused up front — before
// any sealing — so the counter is not burned and the session survives.
func TestSendFrameOversizeReturnsErrFrameTooLarge(t *testing.T) {
	s := &Session{}
	big := make([]byte, secureMaxMessageSize)
	if err := s.SendFrame(big); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("SendFrame(%d bytes) = %v, want ErrFrameTooLarge", len(big), err)
	}
}

// secureSessionPair spins up a wsTestServer whose session has the record
// layer installed, and returns the server-side session plus the client's
// mirror channel for opening what the server seals.
func secureSessionPair(t *testing.T) (*Session, *websocket.Conn, *securechan.Channel, func()) {
	t.Helper()
	key := bytes.Repeat([]byte{0x42}, securechan.KeySize)

	var sess *Session
	var mu sync.Mutex
	c, teardown := wsTestServer(t, noopHandler{}, func(s *Session) {
		ch, err := securechan.NewChannel(key, false)
		if err != nil {
			t.Errorf("NewChannel: %v", err)
			return
		}
		s.EnableSecure(ch)
		mu.Lock()
		sess = s
		mu.Unlock()
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := sess != nil
		mu.Unlock()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if sess == nil {
		teardown()
		t.Fatal("session not captured")
	}

	clientCh, err := securechan.NewChannel(key, true)
	if err != nil {
		teardown()
		t.Fatalf("client NewChannel: %v", err)
	}
	return sess, c, clientCh, teardown
}

// readSealedPlain reads one binary frame from the client conn and opens it.
func readSealedPlain(t *testing.T, c *websocket.Conn, ch *securechan.Channel) []byte {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	frameType, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if frameType != websocket.BinaryMessage {
		t.Fatalf("secure channel carried frame type %d, want binary", frameType)
	}
	plain, err := ch.Open(msg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return plain
}

// TestSendFrameSealedRoundTrip is the core contract: the client receives a
// sealed binary message whose plaintext is FramePrefixJPEG || jpeg, control
// messages interleave on the same counter sequence, and an oversized frame
// in the middle is skipped without desyncing anything.
func TestSendFrameSealedRoundTrip(t *testing.T) {
	sess, c, clientCh, teardown := secureSessionPair(t)
	defer teardown()

	jpeg := bytes.Repeat([]byte{0xD8}, 300*1024)
	if err := sess.SendFrame(jpeg); err != nil {
		t.Fatalf("SendFrame: %v", err)
	}

	plain := readSealedPlain(t, c, clientCh)
	if len(plain) == 0 || plain[0] != FramePrefixJPEG {
		t.Fatalf("frame plaintext starts with %#x, want FramePrefixJPEG (%#x)", plain[:1], FramePrefixJPEG)
	}
	if !bytes.Equal(plain[1:], jpeg) {
		t.Fatal("frame payload corrupted in transit")
	}

	// An oversized frame is refused without burning a counter…
	if err := sess.SendFrame(make([]byte, secureMaxMessageSize)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversize SendFrame = %v, want ErrFrameTooLarge", err)
	}

	// …so a control message and another frame still open cleanly after it.
	if err := sess.Send(MsgPong, nil); err != nil {
		t.Fatalf("Send after frames: %v", err)
	}
	ctrl := readSealedPlain(t, c, clientCh)
	if len(ctrl) == 0 || ctrl[0] != '{' {
		t.Fatalf("control plaintext starts with %#x, want '{'", ctrl[:1])
	}
	env, err := Decode(ctrl)
	if err != nil {
		t.Fatalf("decode control: %v", err)
	}
	if env.Type != MsgPong {
		t.Fatalf("control message type = %s, want %s", env.Type, MsgPong)
	}

	small := bytes.Repeat([]byte{0x11}, 32*1024)
	if err := sess.SendFrame(small); err != nil {
		t.Fatalf("SendFrame after skip: %v", err)
	}
	plain2 := readSealedPlain(t, c, clientCh)
	if plain2[0] != FramePrefixJPEG || !bytes.Equal(plain2[1:], small) {
		t.Fatal("frame after oversize skip did not survive")
	}
}
