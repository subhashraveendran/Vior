// Package network handles device discovery and connection management.
package network

import (
	"encoding/base64"
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// QRCodePlain returns a compact half-block QR code. The previous
// renderer drew "██" per dark module (two full blocks wide) and one
// text row per module — roughly 2× too wide AND full height, which
// overflowed phone-height terminals. ToSmallString packs two module
// rows into each text line via half-block glyphs (▀▄█), halving the
// height while staying scannable, and includes the mandatory quiet
// zone.
func QRCodePlain(url string) (string, error) {
	qr, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return "", fmt.Errorf("qr generate: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(qr.ToSmallString(false))
	sb.WriteString(fmt.Sprintf("\n  %s\n\n", url))
	return sb.String(), nil
}

// QRCodePNG returns a PNG-encoded QR code as bytes.
func QRCodePNG(url string, size int) ([]byte, error) {
	return qrcode.Encode(url, qrcode.Medium, size)
}

// QRCodeDataURL returns a data: URL for embedding in HTML/img tags.
func QRCodeDataURL(url string) (string, error) {
	png, err := QRCodePNG(url, 256)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
