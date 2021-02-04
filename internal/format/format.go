package format

import (
	"fmt"
	"strings"
	"time"
)

// Bytes attaches a unit to the bytes value and makes it human readable.
func Bytes(bytes int64) string {
	if bytes >= 1e12 {
		return fmt.Sprintf("%dTB", bytes/1e12)
	} else if bytes >= 1e9 {
		return fmt.Sprintf("%dGB", bytes/1e9)
	} else if bytes >= 1e6 {
		return fmt.Sprintf("%dMB", bytes/1e6)
	} else if bytes >= 1e3 {
		return fmt.Sprintf("%dKB", bytes/1e3)
	} else {
		return fmt.Sprintf("%dB", bytes)
	}
}

// Filename takes the given filename and rotates it like a carousel
// through a fixed length string of maxLen. See tests for example.
func Filename(fn string, iteration int, maxLen int) string {
	if len(fn) <= maxLen {
		return fn
	}
	out := make([]byte, maxLen)
	padded := fmt.Sprintf("%s%*s", fn, maxLen, " ")
	for i := 0; i < maxLen; i++ {
		if len(fn) <= maxLen {
			out[i] = padded[i]
		} else {
			out[i] = padded[(i+iteration)%len(padded)]
		}
	}
	return string(out)
}

// Progress builds a string of length `width` that indicates the `percent` progress being made by filled █ characters.
func Progress(width int, percent float64) string {
	if width < 5 {
		return fmt.Sprintf("%*s", width, " ")
	}
	effWidth := width - 2 // brackets left and right
	filled := int(float64(effWidth) * percent)
	blank := effWidth - filled
	if percent >= 1.0 {
		return fmt.Sprintf("|%s|", strings.Repeat("█", effWidth))
	} else {
		return fmt.Sprintf("|%s%s|", strings.Repeat("█", filled), strings.Repeat("-", blank))
	}
}

// Speed formats the given bytes per second value to a human readable format.
func Speed(bytesPerS int64) string {
	return fmt.Sprintf("%s/s", Bytes(bytesPerS))
}

// TransferStatus takes the terminal width `twidth` and builds a string occupying the whole width indicating the
// current transfer status.
func TransferStatus(fn string, iteration int, twidth int, p float64, eta time.Duration, bytesPerS int64) string {
	fnStr := Filename(fn, iteration, 16)
	pStr := fmt.Sprintf("%.0f%%", p*100)
	etaStr := fmt.Sprintf("[eta %s]", eta.Round(time.Second))
	speedStr := Speed(bytesPerS)
	progressWidth := twidth - len(fnStr) - len(pStr) - len(etaStr) - len(speedStr) - 4
	progressStr := Progress(progressWidth, p)
	return fmt.Sprintf("%s %s %s %s %s", fnStr, progressStr, pStr, speedStr, etaStr)
}
