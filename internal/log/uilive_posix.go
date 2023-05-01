//go:build !windows
// +build !windows

package log

import (
	"fmt"
	"strings"
)

// clear the line and move the cursor up
var clear = fmt.Sprintf("%c[%dA%c[2K", ESC, 1, ESC)

func (w *UILiveWriter) clearLines() {
	_, _ = fmt.Fprint(w.Out, strings.Repeat(clear, w.lineCount))
}
