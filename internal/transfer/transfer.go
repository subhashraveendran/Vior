// Package transfer handles file transfer between devices.
package transfer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// TransferStatus represents the state of a file transfer.
type TransferStatus int

const (
	StatusPending    TransferStatus = iota
	StatusInProgress
	StatusCompleted
	StatusFailed
)

func (s TransferStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusInProgress:
		return "in-progress"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Progress tracks transfer progress.
type Progress struct {
	FileName     string         `json:"fileName"`
	TotalBytes   int64          `json:"totalBytes"`
	Transferred  int64          `json:"transferred"`
	Status       TransferStatus `json:"status"`
	Error        string         `json:"error,omitempty"`
	StartedAt    time.Time      `json:"startedAt"`
	CompletedAt  *time.Time     `json:"completedAt,omitempty"`
}

// PercentDone returns progress as a percentage.
func (p *Progress) PercentDone() float64 {
	if p.TotalBytes == 0 {
		return 0
	}
	return float64(p.Transferred) / float64(p.TotalBytes) * 100
}

// Transfer defines the interface for file transfer implementations.
type Transfer interface {
	Send(filePath string) error
	Receive(destDir string) error
	Progress() *Progress
}

// TCPTransfer implements Transfer over a TCP connection.
type TCPTransfer struct {
	reader   io.Reader
	writer   io.Writer
	progress *Progress
}

// NewTCPTransfer creates a TCP file transfer handler.
func NewTCPTransfer(r io.Reader, w io.Writer) *TCPTransfer {
	return &TCPTransfer{
		reader: r,
		writer: w,
	}
}

// Send sends a file from local disk to the connected peer.
func (t *TCPTransfer) Send(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	t.progress = &Progress{
		FileName:    filepath.Base(filePath),
		TotalBytes:  fi.Size(),
		Status:      StatusInProgress,
		StartedAt:   time.Now(),
	}

	buf := make([]byte, 64*1024) // 64KB chunks
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := t.writer.Write(buf[:n]); werr != nil {
				t.progress.Status = StatusFailed
				t.progress.Error = werr.Error()
				return fmt.Errorf("write: %w", werr)
			}
			t.progress.Transferred += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.progress.Status = StatusFailed
			t.progress.Error = err.Error()
			return fmt.Errorf("read: %w", err)
		}
	}

	now := time.Now()
	t.progress.CompletedAt = &now
	t.progress.Status = StatusCompleted
	return nil
}

// Receive receives a file from the connected peer and writes to destDir.
func (t *TCPTransfer) Receive(destDir string) error {
	// Read file name length + name + data from stream.
	// Format: [4 bytes: nameLen][nameLen bytes: fileName][data...]
	header := make([]byte, 4)
	if _, err := io.ReadFull(t.reader, header); err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	nameLen := int32(header[0])<<24 | int32(header[1])<<16 | int32(header[2])<<8 | int32(header[3])
	if nameLen <= 0 || nameLen > 4096 {
		return fmt.Errorf("invalid file name length: %d", nameLen)
	}

	nameBuf := make([]byte, nameLen)
	if _, err := io.ReadFull(t.reader, nameBuf); err != nil {
		return fmt.Errorf("read filename: %w", err)
	}
	fileName := string(nameBuf)

	destPath := filepath.Join(destDir, filepath.Base(fileName))
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	t.progress = &Progress{
		FileName:  filepath.Base(fileName),
		Status:    StatusInProgress,
		StartedAt: time.Now(),
	}

	buf := make([]byte, 64*1024)
	for {
		n, err := t.reader.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				t.progress.Status = StatusFailed
				t.progress.Error = werr.Error()
				return fmt.Errorf("write file: %w", werr)
			}
			t.progress.Transferred += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.progress.Status = StatusFailed
			t.progress.Error = err.Error()
			return fmt.Errorf("read: %w", err)
		}
	}

	fi, _ := f.Stat()
	t.progress.TotalBytes = fi.Size()

	now := time.Now()
	t.progress.CompletedAt = &now
	t.progress.Status = StatusCompleted

	fmt.Printf("Received: %s (%d bytes)\n", destPath, t.progress.Transferred)
	return nil
}

// Progress returns the current transfer progress.
func (t *TCPTransfer) Progress() *Progress {
	return t.progress
}
