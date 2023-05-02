package log

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ESC is the ASCII code for escape character
const ESC = 27

var AnsiEscapeCharRegex = regexp.MustCompile(`[\x1b\x9b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]`)

// RefreshInterval is the default refresh interval to update the ui
var RefreshInterval = time.Millisecond

var overFlowHandled bool

var termWidth int

// UILiveWriter is a buffered the writer that updates the terminal. The contents of writer will be flushed on a timed interval or when Flush is called.
type UILiveWriter struct {
	// Out is the writer to write to
	Out io.Writer

	// RefreshInterval is the time the UI sould refresh
	RefreshInterval time.Duration

	ticker *time.Ticker
	tdone  chan bool

	buf       bytes.Buffer
	mtx       *sync.Mutex
	lineCount int
}

type bypass struct {
	writer *UILiveWriter
}

// NewUILive returns a new UILiveWriter with defaults
func NewUILive() *UILiveWriter {
	termWidth, _ = getTermSize()
	if termWidth != 0 {
		overFlowHandled = true
	}

	return &UILiveWriter{
		Out:             Out,
		RefreshInterval: RefreshInterval,

		mtx: &sync.Mutex{},
	}
}

// Flush writes to the out and resets the buffer. It should be called after the last call to Write to ensure that any data buffered in the UILiveWriter is written to output.
// Any incomplete escape sequence at the end is considered complete for formatting purposes.
// An error is returned if the contents of the buffer cannot be written to the underlying output stream
func (w *UILiveWriter) Flush() error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// do nothing if buffer is empty
	if len(w.buf.Bytes()) == 0 {
		return nil
	}
	w.clearLines()

	w.lineCount = 0
	cleaned := AnsiEscapeCharRegex.ReplaceAllString(string(w.buf.Bytes()), "")
	lines := strings.Split(cleaned, "\n")
	for _, line := range lines {
		w.lineCount += 1 + len(line)/termWidth
	}
	w.lineCount -= 1

	_, err := w.Out.Write(w.buf.Bytes())
	w.buf.Reset()
	return err
}

// Start starts the listener in a non-blocking manner
func (w *UILiveWriter) Start() {
	if w.ticker == nil {
		w.ticker = time.NewTicker(w.RefreshInterval)
		w.tdone = make(chan bool)
	}

	go w.Listen()
}

// Stop stops the listener that updates the terminal
func (w *UILiveWriter) Stop() {
	w.Flush()
	w.tdone <- true
	<-w.tdone
}

// Listen listens for updates to the writer's buffer and flushes to the out provided. It blocks the runtime.
func (w *UILiveWriter) Listen() {
	for {
		select {
		case <-w.ticker.C:
			if w.ticker != nil {
				_ = w.Flush()
			}
		case <-w.tdone:
			w.mtx.Lock()
			w.ticker.Stop()
			w.ticker = nil
			w.mtx.Unlock()
			close(w.tdone)
			return
		}
	}
}

// Write save the contents of buf to the writer b. The only errors returned are ones encountered while writing to the underlying buffer.
func (w *UILiveWriter) Write(buf []byte) (n int, err error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.buf.Write(buf)
}

// Bypass creates an io.UILiveWriter which allows non-buffered output to be written to the underlying output
func (w *UILiveWriter) Bypass() io.Writer {
	return &bypass{writer: w}
}

func (b *bypass) Write(p []byte) (int, error) {
	b.writer.mtx.Lock()
	defer b.writer.mtx.Unlock()

	b.writer.clearLines()
	b.writer.lineCount = 0
	return b.writer.Out.Write(p)
}
