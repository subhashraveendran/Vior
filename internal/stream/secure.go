package stream

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/subhashraveendran/vior/internal/handshake"
	"github.com/subhashraveendran/vior/internal/protocol"
	"github.com/subhashraveendran/vior/internal/securechan"
)

// secureHandshakeTimeout bounds each handshake step. It is far tighter than
// helloTimeout because the handshake is two cheap round trips with no user
// interaction — a client that cannot manage it in five seconds is stuck, and
// holding pre-auth connections open is exactly what an attacker would want.
const secureHandshakeTimeout = 5 * time.Second

// SecurityMode controls whether the encrypted channel is required, offered,
// or disabled.
type SecurityMode int

const (
	// SecurePreferred accepts both secure and legacy cleartext clients.
	// This is the rollout default: it lets already-installed clients keep
	// working while new ones negotiate encryption.
	SecurePreferred SecurityMode = iota

	// SecureRequired rejects any client that does not complete the
	// handshake. This is the intended end state.
	SecureRequired

	// SecureOff disables the handshake entirely. Debugging escape hatch —
	// it should be deleted before 1.0 rather than allowed to ossify into
	// a permanent downgrade switch.
	SecureOff
)

var (
	securityModeMu sync.RWMutex
	securityMode   = SecurePreferred
)

// SetSecurityMode sets the transport-security policy.
func SetSecurityMode(m SecurityMode) {
	securityModeMu.Lock()
	securityMode = m
	securityModeMu.Unlock()
}

// GetSecurityMode returns the active transport-security policy.
func GetSecurityMode() SecurityMode {
	securityModeMu.RLock()
	defer securityModeMu.RUnlock()
	return securityMode
}

// String renders the mode for logs and the /info payload.
func (m SecurityMode) String() string {
	switch m {
	case SecureRequired:
		return "required"
	case SecureOff:
		return "off"
	default:
		return "preferred"
	}
}

// negotiateSecure reads the client's first frame and either completes the
// secure handshake or admits a legacy cleartext client, according to policy.
//
// It returns the hello message either way, because the first frame is
// necessarily consumed here: a legacy client's first frame IS the hello, so
// this function owns reading it rather than leaving a half-read stream for
// WaitForHello. The returned frame token is empty for cleartext sessions.
//
// The handshake runs before hello by design, so the pair code — and every
// keystroke, file chunk and control message after it — is inside the
// encrypted channel rather than in front of it.
func (s *MJPEGServer) negotiateSecure(session *protocol.Session) (hello *protocol.HelloMessage, frameToken string, err error) {
	mode := GetSecurityMode()

	if mode == SecureOff {
		h, err := session.WaitForHello()
		return h, "", err
	}

	env, err := session.ReadEnvelope(secureHandshakeTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("reading first frame: %w", err)
	}

	switch env.Type {
	case protocol.MsgSecureInit:
		return s.completeHandshake(session, env)

	case protocol.MsgHello:
		if mode == SecureRequired {
			session.Send(protocol.MsgError, &protocol.ErrorMessage{
				Code:    protocol.ErrCodeUpgradeRequired,
				Message: "This desktop requires an encrypted connection. Update the Vior app and reconnect by scanning the QR code.",
			})
			return nil, "", fmt.Errorf("client opened in cleartext but policy is %s", mode)
		}
		h, err := protocol.DecodeData[protocol.HelloMessage](env)
		if err != nil {
			return nil, "", fmt.Errorf("decode hello: %w", err)
		}
		// WaitForHello would normally record this; the legacy path
		// consumed the frame here instead, so set it explicitly.
		session.Hello = h
		log.Printf("stream: client connected WITHOUT encryption [%s] — payloads are in cleartext", session.ID)
		return h, "", nil

	default:
		return nil, "", fmt.Errorf("expected %s or %s, got %s", protocol.MsgSecureInit, protocol.MsgHello, env.Type)
	}
}

// completeHandshake runs the responder side and, on success, installs the
// record layer and reads the now-sealed hello.
func (s *MJPEGServer) completeHandshake(session *protocol.Session, env *protocol.Envelope) (*protocol.HelloMessage, string, error) {
	initMsg, err := protocol.DecodeData[protocol.SecureInitMessage](env)
	if err != nil {
		return nil, "", fmt.Errorf("decode secure-init: %w", err)
	}

	responder, err := handshake.NewResponder(ChannelSecret())
	if err != nil {
		return nil, "", fmt.Errorf("responder: %w", err)
	}

	resp, err := responder.Respond(&handshake.Init{
		Version: initMsg.Version,
		PubKey:  initMsg.PubKey,
		Nonce:   initMsg.Nonce,
	})
	if err != nil {
		s.sendSecureFailure(session, err)
		return nil, "", fmt.Errorf("handshake respond: %w", err)
	}

	if err := session.Send(protocol.MsgSecureResp, &protocol.SecureRespMessage{
		PubKey: resp.PubKey,
		Nonce:  resp.Nonce,
		MAC:    resp.MAC,
	}); err != nil {
		return nil, "", fmt.Errorf("send secure-resp: %w", err)
	}

	confirmEnv, err := session.ReadEnvelope(secureHandshakeTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("reading secure-confirm: %w", err)
	}
	if confirmEnv.Type != protocol.MsgSecureConfirm {
		return nil, "", fmt.Errorf("expected %s, got %s", protocol.MsgSecureConfirm, confirmEnv.Type)
	}
	confirmMsg, err := protocol.DecodeData[protocol.SecureConfirmMessage](confirmEnv)
	if err != nil {
		return nil, "", fmt.Errorf("decode secure-confirm: %w", err)
	}

	if err := responder.Finish(&handshake.Confirm{MAC: confirmMsg.MAC}); err != nil {
		// The peer could not prove knowledge of the channel secret: a
		// stale QR code, or someone in the middle. Never distinguish
		// the two to the client.
		s.sendSecureFailure(session, err)
		return nil, "", fmt.Errorf("handshake confirm: %w", err)
	}

	key, err := responder.SessionKey()
	if err != nil {
		return nil, "", fmt.Errorf("session key: %w", err)
	}
	// The server is the responder, so its direction keys are the mirror
	// of the client's.
	ch, err := securechan.NewChannel(key, false)
	if err != nil {
		return nil, "", fmt.Errorf("record layer: %w", err)
	}
	token, err := handshake.FrameToken(key)
	if err != nil {
		return nil, "", fmt.Errorf("frame token: %w", err)
	}

	session.EnableSecure(ch)
	log.Printf("stream: secure channel established [%s]", session.ID)

	// First sealed message: proves the channel works end-to-end and
	// hands over the frame-endpoint token, which must never travel in
	// cleartext.
	if err := session.Send(protocol.MsgSecureReady, &protocol.SecureReadyMessage{FrameToken: token}); err != nil {
		return nil, "", fmt.Errorf("send secure-ready: %w", err)
	}

	hello, err := session.WaitForHello()
	if err != nil {
		return nil, "", fmt.Errorf("sealed hello: %w", err)
	}
	return hello, token, nil
}

// sendSecureFailure reports a handshake failure without revealing which step
// failed or whether the secret was close. The client only needs to know to
// re-scan.
func (s *MJPEGServer) sendSecureFailure(session *protocol.Session, cause error) {
	log.Printf("stream: secure handshake failed [%s]: %v", session.ID, cause)
	session.Send(protocol.MsgError, &protocol.ErrorMessage{
		Code:    protocol.ErrCodeSecureFailed,
		Message: "Could not establish an encrypted connection. Scan the QR code on the desktop again.",
	})
}
