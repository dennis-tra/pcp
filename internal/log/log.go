package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

type Level uint8

const (
	DebugLevel Level = iota
	InfoLevel
	WarningLevel
	ErrorLevel
)

var level Level

func SetLevel(l Level) {
	level = l
}

// Out represents the writer to print the log messages to.
// This is used for tests.
var Out io.Writer = os.Stderr

var tWidth int

func init() {
	level = InfoLevel

	var err error
	tWidth, _, err = terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		tWidth, _, err = terminal.GetSize(int(os.Stderr.Fd()))
		if err != nil {
			tWidth = 80
		}
	}
}

func printTimestamp() {
	if level > DebugLevel {
		return
	}
	fmt.Printf("[%s] ", time.Now().Format(time.RFC3339))
}

func Info(a ...interface{}) {
	if level > InfoLevel {
		return
	}
	printTimestamp()
	fmt.Fprint(Out, a...)
}

func Infoln(a ...interface{}) {
	if level > InfoLevel {
		return
	}
	printTimestamp()
	fmt.Fprintln(Out, a...)
}

func Infor(format string, a ...interface{}) {
	if level > InfoLevel {
		return
	}

	if level > DebugLevel {
		blank := fmt.Sprintf("\r%s\r", strings.Repeat(" ", tWidth))
		printTimestamp()
		fmt.Fprint(Out, fmt.Sprintf("%s%s", blank, fmt.Sprintf(format, a...)))
	} else {
		printTimestamp()
		fmt.Fprintln(Out, fmt.Sprintf(strings.TrimSpace(format), a...))
	}
}

func Infof(format string, a ...interface{}) {
	if level > InfoLevel {
		return
	}
	printTimestamp()
	fmt.Fprintf(Out, format, a...)
}

func Debug(a ...interface{}) {
	if level > DebugLevel {
		return
	}
	printTimestamp()
	fmt.Fprint(Out, a...)
}

func Debugln(a ...interface{}) {
	if level > DebugLevel {
		return
	}
	printTimestamp()
	fmt.Fprintln(Out, a...)
}

func Debugf(format string, a ...interface{}) {
	if level > DebugLevel {
		return
	}
	printTimestamp()
	fmt.Fprintf(Out, format, a...)
}

func Warning(a ...interface{}) {
	if level > WarningLevel {
		return
	}
	printTimestamp()
	fmt.Fprint(Out, a...)
}

func Warningln(a ...interface{}) {
	if level > WarningLevel {
		return
	}
	printTimestamp()
	fmt.Fprintln(Out, a...)
}

func Warningf(format string, a ...interface{}) {
	if level > WarningLevel {
		return
	}
	printTimestamp()
	fmt.Fprintf(Out, format, a...)
}

func Error(a ...interface{}) {
	printTimestamp()
	fmt.Fprint(Out, a...)
}

func Errorln(a ...interface{}) {
	printTimestamp()
	fmt.Fprintln(Out, a...)
}

func Errorf(format string, a ...interface{}) {
	printTimestamp()
	fmt.Fprintf(Out, format, a...)
}
