package log

var SpinnerChars = []string{"⠋ ", "⠙ ", "⠹ ", "⠸ ", "⠼ ", "⠴ ", "⠦ ", "⠧ ", "⠇ ", "⠏ "}

const (
	EscReset    = "\033[0m"
	FontBold    = "\033[1m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[37m"
	ColorWhite  = "\033[97m"
)

func Bold(s string) string {
	return FontBold + s + EscReset
}

func Red(s string) string {
	return ColorRed + s + EscReset
}

func Green(s string) string {
	return ColorGreen + s + EscReset
}

func Yellow(s string) string {
	return ColorYellow + s + EscReset
}

func Blue(s string) string {
	return ColorBlue + s + EscReset
}

func Purple(s string) string {
	return ColorPurple + s + EscReset
}

func Cyan(s string) string {
	return ColorCyan + s + EscReset
}

func Gray(s string) string {
	return ColorGray + s + EscReset
}

func White(s string) string {
	return ColorWhite + s + EscReset
}
