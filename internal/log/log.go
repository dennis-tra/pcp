package log

import (
	"fmt"
	"io"
	"os"
)

// Out represents the writer to print the log messages to.
// This is used for tests.
var Out io.Writer = os.Stderr

func Info(a ...interface{}) {
	fmt.Fprint(Out, a...)
}

func Infoln(a ...interface{}) {
	fmt.Fprintln(Out, a...)
}

func Infof(format string, a ...interface{}) {
	fmt.Fprintf(Out, format, a...)
}
