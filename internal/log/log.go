package log

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

// Out represents the writer to print the log messages to.
// This is used for tests.
var Out io.Writer = os.Stderr

var tWidth int

func init() {
	var err error
	tWidth, _, err = terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		tWidth, _, err = terminal.GetSize(int(os.Stderr.Fd()))
		if err != nil {
			tWidth = 80
		}
	}
}

func Info(a ...interface{}) {
	fmt.Fprint(Out, a...)
}

func Infoln(a ...interface{}) {
	fmt.Fprintln(Out, a...)
}

func Infor(format string, a ...interface{}) {
	blank := fmt.Sprintf("\r%s\r", strings.Repeat(" ", tWidth))
	fmt.Fprint(Out, fmt.Sprintf("%s%s", blank, fmt.Sprintf(format, a...)))
}

func Infof(format string, a ...interface{}) {
	fmt.Fprintf(Out, format, a...)
}

func Debug(a ...interface{}) {
	fmt.Fprint(Out, a...)
}

func Debugln(a ...interface{}) {
	fmt.Fprintln(Out, a...)
}

func Debugf(format string, a ...interface{}) {
	fmt.Fprintf(Out, format, a...)
}

func Warning(a ...interface{}) {
	fmt.Fprint(Out, a...)
}

func Warningln(a ...interface{}) {
	fmt.Fprintln(Out, a...)
}

func Warningf(format string, a ...interface{}) {
	fmt.Fprintf(Out, format, a...)
}

func Error(a ...interface{}) {
	fmt.Fprint(Out, a...)
}

func Errorln(a ...interface{}) {
	fmt.Fprintln(Out, a...)
}

func Errorf(format string, a ...interface{}) {
	fmt.Fprintf(Out, format, a...)
}
