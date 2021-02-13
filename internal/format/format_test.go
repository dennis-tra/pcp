package format

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{1000, "1KB"},
	}

	for _, tt := range tests {
		run := fmt.Sprintf("Formatting %d", tt.bytes)
		t.Run(run, func(t *testing.T) {
			s := Bytes(tt.bytes)
			assert.Equal(t, tt.want, s)
		})
	}
}

func TestFormatFilename(t *testing.T) {
	tests := []struct {
		name      string
		iteration int
		maxLen    int
		want      string
	}{
		{"pcp.go", 0, 3, "pcp"},
		{"pcp.go", 2, 3, "p.g"},
		{"pcp.go", 6, 3, "   "},
		{"pcp.go", 8, 3, " pc"},
		{"test-file.txt", 0, 16, "test-file.txt"},
		{"test-file.txt", 8, 16, "test-file.txt"},
		{"test-file.txt", 16, 16, "test-file.txt"},
		{"test-file.txt", 20, 16, "test-file.txt"},
		{"", 0, 16, ""},
		{"a-really-long-test-file.txt", 0, 16, "a-really-long-te"},
		{"a-really-long-test-file.txt", 1, 16, "-really-long-tes"},
		{"a-really-long-test-file.txt", 11, 16, "ng-test-file.txt"},
		{"a-really-long-test-file.txt", 12, 16, "g-test-file.txt "},
		{"a-really-long-test-file.txt", 16, 16, "st-file.txt     "},
		{"a-really-long-test-file.txt", 27, 16, "                "},
		{"a-really-long-test-file.txt", 28, 16, "               a"},
		{"a-really-long-test-file.txt", 33, 16, "          a-real"},
	}

	for _, tt := range tests {
		run := fmt.Sprintf("Formatting %s (i: %d, len: %d)", tt.name, tt.iteration, tt.maxLen)
		t.Run(run, func(t *testing.T) {
			s := Filename(tt.name, tt.iteration, tt.maxLen)
			assert.Equal(t, tt.want, s, "Expected filename %q (i: %d, len: %d) to equal %q, got: %q", tt.name, tt.iteration, tt.maxLen, tt.want, s)
		})
	}
}

func TestFormatProgress(t *testing.T) {
	tests := []struct {
		width   int
		percent float64
		want    string
	}{
		{10, 0.0, "|--------|"},
		{10, 0.5, "|████----|"},
		{10, 0.999, "|███████-|"},
		{10, 1.0, "|████████|"},
		{2, 0.1, "  "},
		{5, 0.2, "|---|"},
		{5, 0.35, "|█--|"},
		{5, 0.9, "|██-|"},
		{5, 1.0, "|███|"},
	}

	for _, tt := range tests {
		run := fmt.Sprintf("Formatting Progress (width: %d, percent: %.1f)", tt.width, tt.percent)
		t.Run(run, func(t *testing.T) {
			s := Progress(tt.width, tt.percent)
			assert.Equal(t, tt.want, s, "Expected width: %d, percent: %d to equal %q, got: %q", tt.width, tt.percent, tt.want, s)
		})
	}
}

func TestFormatTransferStatus(t *testing.T) {
	tests := []struct {
		filename  string
		iteration int
		twidth    int
		percent   float64
		eta       time.Duration
		bytesPerS int64
		want      string
	}{
		{
			filename:  "test.txt",
			iteration: 0,
			twidth:    80,
			percent:   0.4,
			eta:       time.Minute,
			bytesPerS: 235000,
			want:      "test.txt |██████████████████----------------------------| 40% 235KB/s [eta 1m0s]",
		},
		{
			filename:  "a-really-long-test-file.txt",
			iteration: 334,
			twidth:    80,
			percent:   0.6,
			eta:       time.Minute + 5*time.Second,
			bytesPerS: 87000000,
			want:      "          a-real |███████████████████████----------------| 60% 87MB/s [eta 1m5s]",
		},
	}

	for _, tt := range tests {
		run := fmt.Sprintf("Formatting Transfer Status")
		t.Run(run, func(t *testing.T) {
			s := TransferStatus(tt.filename, tt.iteration, tt.twidth, tt.percent, tt.eta, tt.bytesPerS)
			assert.Equal(t, tt.want, s)
		})
	}
}
