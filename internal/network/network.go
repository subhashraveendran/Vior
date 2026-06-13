// Package network handles device discovery and connection management.
package network

import (
	"encoding/base64"
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// QRCodePlain returns an ASCII-art QR code using plain characters
// (no ANSI codes, safe for all terminals).
func QRCodePlain(url string) (string, error) {
	qr, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return "", fmt.Errorf("qr generate: %w", err)
	}

	bitmap := qr.Bitmap()
	var sb strings.Builder
	sb.WriteString("\n")

	for _, row := range bitmap {
		sb.WriteString("  ")
		for _, col := range row {
			if col {
				sb.WriteString("██")
			} else {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}
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
