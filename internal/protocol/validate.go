package protocol

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Inbound message validation.
//
// Every message a client sends is checked here before it reaches a
// MessageHandler. The rules are deliberately explicit: each field has an
// enum, a pattern or a numeric bound, so a handler never has to wonder
// whether a value came off the wire unchecked.
//
// Two kinds of outcome:
//
//   - A structural violation (bad enum, malformed id, NaN coordinate,
//     out-of-range dimension, unknown key name) returns an error wrapping
//     ErrInvalidMessage. ReadLoop drops the message.
//   - A cosmetic field that is merely too long or carries control
//     characters (device name, platform label, reject reason, thumbnail
//     preview) is normalised in place — trimmed, truncated or cleared —
//     instead of rejecting the whole message. Refusing a connection
//     because a phone model string is 70 characters long, or failing a
//     file transfer because its optional thumbnail is too large, would be
//     a user-visible regression with no security benefit.
//
// Error messages name the field and the rule, never the value: a rejected
// hello may carry a pair code, and rejected chunks carry file contents.

// ErrInvalidMessage is wrapped by every validation failure.
var ErrInvalidMessage = errors.New("protocol: invalid message")

// Bounds for client-supplied values.
const (
	// MaxClientDimension bounds a single client-supplied width or height.
	// It mirrors maxClientDimension in internal/session (which imports
	// this package, so it cannot be referenced from here); Configure
	// re-checks the same bound before allocating anything.
	MaxClientDimension = 16384

	// MinDPR and MaxDPR bound the device pixel ratio. Zero means "use the
	// default" and is passed through unchanged.
	MinDPR = 0.5
	MaxDPR = 5.0

	// MaxNameRunes and MaxPlatformRunes bound the cosmetic labels a
	// client reports in hello. Longer values are truncated.
	MaxNameRunes     = 64
	MaxPlatformRunes = 32

	// MaxDeviceIDLen bounds hello.deviceId.
	MaxDeviceIDLen = 80

	// MaxPairCodeLen bounds hello.pairCode. Pair codes are 4–8 digits
	// (see pairOverrideRe in internal/stream); anything longer cannot
	// match and is refused before it reaches the constant-time compare.
	MaxPairCodeLen = 8

	// MaxInputMagnitude bounds |x|, |y|, |dx| and |dy| on input events.
	// Four times the largest display edge covers every legitimate touch
	// coordinate and every wheel delta a browser produces; the input
	// layer clamps further.
	MaxInputMagnitude = 4 * MaxClientDimension

	// MaxKeyLen bounds the raw key string before it is parsed.
	MaxKeyLen = 64

	// MaxFileNameBytes bounds file-offer names (the common filesystem
	// limit for a single path component).
	MaxFileNameBytes = 255

	// MaxFileSize is the largest file a peer may offer. It mirrors
	// filetransfer.MaxDownloadSize (filetransfer imports this package);
	// a test in this package's external test suite keeps the two equal.
	MaxFileSize int64 = 2 * 1024 * 1024 * 1024

	// MaxMimeTypeLen bounds file-offer mimeType.
	MaxMimeTypeLen = 128

	// MaxPreviewBytes bounds the inline thumbnail on a file offer. An
	// oversized or malformed preview is cleared, not rejected: the mobile
	// app already caps its thumbnails at 32 KiB, but the browser client
	// sends the whole image as a data: URL.
	MaxPreviewBytes = 48 * 1024

	// MaxChunkDataLen bounds the base64 payload of one file-chunk.
	// Clients send 48 KiB raw (64 KiB encoded); 96 KiB leaves headroom.
	MaxChunkDataLen = 96 * 1024

	// MaxReasonBytes bounds reject reasons. Longer reasons are truncated.
	MaxReasonBytes = 256
)

// transferIDRe matches transfer ids: lowercase hex, the shape
// internal/filetransfer generates and validates (transferIDRe there) and the
// mobile client filters on (VALID_ID in files.ts).
var transferIDRe = regexp.MustCompile(`^[a-f0-9]{8,64}$`)

// deviceIDRe matches hello.deviceId. Clients send "mob-<uuid>",
// "web-<uuid>" or a base36 fallback; the pattern admits those and anything
// similarly inert, and refuses whitespace, quotes, slashes and control
// characters — the id is a trust-store key and appears in logs.
var deviceIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

func invalid(field, rule string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalidMessage, field, rule)
}

// Validate checks and normalises one decoded inbound message. It accepts a
// pointer to any client→server message struct in this package and returns
// an error wrapping ErrInvalidMessage when the message must be dropped. A
// type it does not know is an error too, so a new message type cannot reach
// a handler without someone writing its rules.
func Validate(msg any) error {
	switch m := msg.(type) {
	case *HelloMessage:
		return ValidateHello(m)
	case *ResizeMessage:
		return ValidateResize(m)
	case *InputMessage:
		return ValidateInput(m)
	case *FileOfferMessage:
		return ValidateFileOffer(m)
	case *FileChunkMessage:
		return ValidateFileChunk(m)
	case *FileAcceptMessage:
		return validateID("id", m.ID)
	case *FileRejectMessage:
		if err := validateID("id", m.ID); err != nil {
			return err
		}
		m.Reason = clampText(m.Reason, MaxReasonBytes)
		return nil
	case *FileCompleteMessage:
		return ValidateFileComplete(m)
	case *DownloadAcceptMessage:
		return validateID("id", m.ID)
	case *DownloadRejectMessage:
		if err := validateID("id", m.ID); err != nil {
			return err
		}
		m.Reason = clampText(m.Reason, MaxReasonBytes)
		return nil
	case *DownloadCompleteMessage:
		return validateID("id", m.ID)
	default:
		return fmt.Errorf("%w: no validator for %T", ErrInvalidMessage, msg)
	}
}

// DecodeHello decodes and validates a hello envelope. Use it anywhere a
// hello is read off the wire instead of DecodeData[HelloMessage], so the
// pre-admission hello gets the same checks as every later message.
func DecodeHello(env *Envelope) (*HelloMessage, error) {
	if env == nil || env.Type != MsgHello {
		return nil, invalid("type", "expected hello")
	}
	h, err := DecodeData[HelloMessage](env)
	if err != nil {
		return nil, fmt.Errorf("%w: hello: malformed JSON", ErrInvalidMessage)
	}
	if err := ValidateHello(h); err != nil {
		return nil, err
	}
	return h, nil
}

// ValidateHello checks a hello and normalises its cosmetic fields in place.
func ValidateHello(h *HelloMessage) error {
	if err := validateDims(h.Width, h.Height); err != nil {
		return err
	}
	dpr, err := normaliseDPR(h.DPR)
	if err != nil {
		return err
	}
	h.DPR = dpr
	switch h.Mode {
	case "", "extend", "mirror":
	default:
		return invalid("mode", "must be extend or mirror")
	}
	switch h.Intent {
	case "", "display", "remote", "files":
	default:
		return invalid("intent", "must be display, remote or files")
	}
	// Never echo the pair code: only its shape is checked here, and the
	// error text says nothing about what was sent.
	pc := strings.TrimSpace(h.PairCode)
	if len(pc) > MaxPairCodeLen || !allDigits(pc) {
		return invalid("pairCode", "must be at most 8 digits")
	}
	h.PairCode = pc
	if h.DeviceID != "" {
		if len(h.DeviceID) > MaxDeviceIDLen || !deviceIDRe.MatchString(h.DeviceID) {
			return invalid("deviceId", "must match [A-Za-z0-9][A-Za-z0-9._:-]{0,79}")
		}
	}
	h.Name = clampLabel(h.Name, MaxNameRunes)
	h.Platform = clampLabel(h.Platform, MaxPlatformRunes)
	return nil
}

// ValidateResize checks a resize with the same dimension, DPR and mode rules
// as hello.
func ValidateResize(r *ResizeMessage) error {
	if err := validateDims(r.Width, r.Height); err != nil {
		return err
	}
	dpr, err := normaliseDPR(r.DPR)
	if err != nil {
		return err
	}
	r.DPR = dpr
	switch r.Mode {
	case "", "extend", "mirror":
	default:
		return invalid("mode", "must be extend or mirror")
	}
	return nil
}

// ValidateInput checks an input event. Key names are normalised in place to
// their canonical spelling (see NormalizeKey).
func ValidateInput(m *InputMessage) error {
	for _, f := range [...]struct {
		name string
		v    float64
	}{{"x", m.X}, {"y", m.Y}, {"dx", m.DX}, {"dy", m.DY}} {
		if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
			return invalid(f.name, "must be finite")
		}
		if math.Abs(f.v) > MaxInputMagnitude {
			return invalid(f.name, "out of range")
		}
	}
	switch m.Event {
	case "touch":
		switch m.Action {
		case "down", "move", "up":
			return nil
		}
		return invalid("action", "touch action must be down, move or up")
	case "mouse":
		switch m.Action {
		case "move", "click", "rightclick", "middleclick":
			return nil
		}
		return invalid("action", "mouse action must be move, click, rightclick or middleclick")
	case "scroll":
		// The browser client's stream pane sends action "scroll"; every
		// other sender omits it.
		switch m.Action {
		case "", "scroll":
			return nil
		}
		return invalid("action", "scroll action must be empty or scroll")
	case "key":
		// Key events are press-and-release in one message; the handler
		// ignores action. "down" is tolerated for clients that label it.
		switch m.Action {
		case "", "down":
		default:
			return invalid("action", "key action must be empty or down")
		}
		k, ok := NormalizeKey(m.Key)
		if !ok {
			return invalid("key", "not an allowed key name")
		}
		m.Key = k
		return nil
	default:
		return invalid("event", "must be touch, mouse, scroll or key")
	}
}

// ValidateFileOffer checks a file offer. An oversized or malformed preview
// is cleared rather than failing the offer.
func ValidateFileOffer(m *FileOfferMessage) error {
	if err := validateID("id", m.ID); err != nil {
		return err
	}
	if len(m.Name) > MaxFileNameBytes {
		return invalid("name", "longer than 255 bytes")
	}
	if strings.ContainsAny(m.Name, "/\\\x00") || m.Name == "." || m.Name == ".." {
		return invalid("name", "must not contain path separators or NUL")
	}
	if m.Size <= 0 || m.Size > MaxFileSize {
		return invalid("size", "must be between 1 byte and 2 GiB")
	}
	if len(m.MimeType) > MaxMimeTypeLen {
		return invalid("mimeType", "longer than 128 bytes")
	}
	if !validPreview(m.Preview) {
		m.Preview = ""
	}
	return nil
}

// ValidateFileChunk checks a file chunk's id, offset and payload shape. The
// payload is never logged or echoed.
func ValidateFileChunk(m *FileChunkMessage) error {
	if err := validateID("id", m.ID); err != nil {
		return err
	}
	if m.Offset < 0 || m.Offset > MaxFileSize {
		return invalid("offset", "out of range")
	}
	if len(m.Data) > MaxChunkDataLen {
		return invalid("data", "longer than 96 KiB")
	}
	if !isBase64(m.Data) {
		return invalid("data", "must be standard base64")
	}
	return nil
}

// ValidateFileComplete checks a completion notice. The hash is optional
// (clients without WebCrypto send ""), otherwise 64 hex digits, normalised to
// lower case to match what the receiver computes.
func ValidateFileComplete(m *FileCompleteMessage) error {
	if err := validateID("id", m.ID); err != nil {
		return err
	}
	if m.Hash == "" {
		return nil
	}
	if len(m.Hash) != 64 || !isHex(m.Hash) {
		return invalid("hash", "must be empty or 64 hex digits")
	}
	m.Hash = strings.ToLower(m.Hash)
	return nil
}

// ValidTransferID reports whether id has the shape of a transfer id.
func ValidTransferID(id string) bool { return transferIDRe.MatchString(id) }

func validateID(field, id string) error {
	if !ValidTransferID(id) {
		return invalid(field, "must be 8-64 lowercase hex digits")
	}
	return nil
}

func validateDims(w, h int) error {
	if w < 1 || w > MaxClientDimension {
		return invalid("width", "must be 1-16384")
	}
	if h < 1 || h > MaxClientDimension {
		return invalid("height", "must be 1-16384")
	}
	return nil
}

// normaliseDPR rejects a DPR that is not a finite, non-negative number, and
// clamps a positive one into [MinDPR, MaxDPR]. Clamping rather than
// rejecting matters for the browser client: page zoom moves
// devicePixelRatio well outside the hardware range (25% zoom on a 1x screen
// reports 0.25; 500% on a 2x screen reports 10), and refusing the
// connection for that would be a regression.
func normaliseDPR(d float64) (float64, error) {
	if math.IsNaN(d) || math.IsInf(d, 0) || d < 0 {
		return 0, invalid("dpr", "must be a finite non-negative number")
	}
	switch {
	case d == 0:
		return 0, nil
	case d < MinDPR:
		return MinDPR, nil
	case d > MaxDPR:
		return MaxDPR, nil
	}
	return d, nil
}

// clampLabel strips control and format characters (which would let a
// client forge log lines or hide text) and truncates to max runes.
func clampLabel(s string, max int) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		if n == max {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// clampText strips control characters and truncates to at most max bytes on
// a rune boundary.
func clampText(s string, max int) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		if b.Len()+utf8.RuneLen(r) > max {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// isBase64 reports whether s uses only the standard base64 alphabet with
// padding only at the end. It does not decode — the receiver does that —
// it only refuses payloads that could never decode.
func isBase64(s string) bool {
	end := len(s)
	for end > 0 && s[end-1] == '=' {
		end--
	}
	if len(s)-end > 2 {
		return false
	}
	for i := 0; i < end; i++ {
		c := s[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return false
		}
	}
	return true
}

// previewImageTypes are the data: URL image subtypes a preview may carry.
// SVG is deliberately absent: it is a document format that can embed
// script, and the preview is rendered by the desktop webview.
var previewImageTypes = map[string]bool{
	"png": true, "jpeg": true, "jpg": true, "gif": true, "webp": true,
	"bmp": true, "avif": true, "heic": true, "heif": true,
}

// validPreview reports whether p is empty, bare base64, or a base64
// data:image/<raster>;base64, URL, within MaxPreviewBytes.
func validPreview(p string) bool {
	if p == "" {
		return true
	}
	if len(p) > MaxPreviewBytes {
		return false
	}
	if !strings.HasPrefix(p, "data:") {
		return isBase64(p)
	}
	rest, ok := strings.CutPrefix(p, "data:image/")
	if !ok {
		return false
	}
	subtype, payload, ok := strings.Cut(rest, ";base64,")
	if !ok || !previewImageTypes[strings.ToLower(subtype)] {
		return false
	}
	return isBase64(payload)
}

// ── Key allowlist ───────────────────────────────────────────────────

// namedKeys maps the lower-cased form of every named key to its canonical
// spelling. The set is the intersection of what clients send and what the
// platform controllers implement: darwinKeyCodes (input_darwin.go) and
// namedVK (input_windows.go) share this vocabulary; Linux passes names to
// XStringToKeysym, which knows the canonical spellings chosen here
// (BackSpace, Return, Escape, Delete, Tab, Home, End, Up/Down/Left/Right,
// F1–F12). Aliases (Backspace, Enter, Esc, Del) normalise to the canonical
// form, which every platform table accepts.
var namedKeys = func() map[string]string {
	canon := []string{
		"BackSpace", "Return", "Tab", "Escape", "Space", "Delete",
		"Home", "End", "PageUp", "PageDown",
		"Up", "Down", "Left", "Right",
		"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12",
	}
	m := make(map[string]string, len(canon)+4)
	for _, k := range canon {
		m[strings.ToLower(k)] = k
	}
	m["enter"] = "Return"
	m["esc"] = "Escape"
	m["del"] = "Delete"
	return m
}()

// modifierKeys maps lower-cased modifier names to canonical spelling. The
// set is Cmd, Ctrl, Alt, Shift, Meta, Super, Win and Option, plus the
// spelled-out aliases Command, Control and Opt that both the darwin and
// windows controllers already accept.
var modifierKeys = map[string]string{
	"cmd": "Cmd", "command": "Cmd",
	"ctrl": "Ctrl", "control": "Ctrl",
	"alt": "Alt",
	"shift": "Shift",
	"meta":  "Meta",
	"super": "Super",
	"win":   "Win",
	"option": "Option", "opt": "Option",
}

// NamedKeys returns the canonical named keys accepted in key events, for
// documentation and tests.
func NamedKeys() []string {
	seen := make(map[string]bool)
	var out []string
	for _, v := range namedKeys {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// NormalizeKey validates a key-event key against the explicit allowlist and
// returns its canonical form. Accepted shapes:
//
//   - a single printable character (unicode.IsGraphic: letters, digits,
//     punctuation, symbols, marks and space characters), returned
//     unchanged — case is meaningful for typed text;
//   - a named key (see namedKeys), case-insensitive, returned canonical;
//   - a chord Mod+…+Key with one to four distinct modifiers from
//     modifierKeys and a final named key or single printable character.
//     Modifiers and named keys are canonicalised; a printable final key
//     keeps its case.
//
// A lone modifier, an empty chord tail ("Cmd+"), a repeated modifier or
// anything else returns ok=false.
func NormalizeKey(key string) (string, bool) {
	if key == "" || len(key) > MaxKeyLen || !utf8.ValidString(key) {
		return "", false
	}
	if k, ok := singleKey(key); ok {
		return k, true
	}
	// "+" on its own was handled above as a printable rune; anything else
	// containing "+" must be a chord.
	parts := strings.Split(key, "+")
	if len(parts) < 2 || len(parts) > 5 {
		return "", false
	}
	tail := parts[len(parts)-1]
	mods := parts[:len(parts)-1]
	// "Cmd++" splits to ["Cmd", "", ""]: the final key is "+" itself.
	if tail == "" && len(mods) >= 2 && mods[len(mods)-1] == "" {
		tail = "+"
		mods = mods[:len(mods)-1]
	}
	final, ok := singleKey(tail)
	if !ok {
		return "", false
	}
	var b strings.Builder
	seen := make(map[string]bool, len(mods))
	for _, m := range mods {
		c, ok := modifierKeys[strings.ToLower(m)]
		if !ok || seen[c] {
			return "", false
		}
		seen[c] = true
		b.WriteString(c)
		b.WriteByte('+')
	}
	b.WriteString(final)
	return b.String(), true
}

// singleKey accepts one printable rune (unchanged) or a named key
// (canonicalised).
func singleKey(s string) (string, bool) {
	if r, size := utf8.DecodeRuneInString(s); size == len(s) && size > 0 {
		if r != utf8.RuneError && unicode.IsGraphic(r) || r == utf8.RuneError && s == "�" {
			return s, true
		}
		return "", false
	}
	if c, ok := namedKeys[strings.ToLower(s)]; ok {
		return c, true
	}
	return "", false
}
