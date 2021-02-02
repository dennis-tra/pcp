package commons

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func TerminalWidth() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output() // returns a string like: 50 232
	if err != nil {
		return 80
	}

	parts := strings.Split(strings.TrimSpace(string(out)), " ")
	if len(parts) != 2 {
		return 80
	}

	width, err := strconv.Atoi(parts[1])
	if len(parts) != 2 {
		return 80
	}

	return width
}
