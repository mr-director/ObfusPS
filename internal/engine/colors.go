package engine

import (
	"os"
)

// ANSI color codes for terminal output (Windows 10+, Linux, macOS).
// Automatically disabled when stderr is not a terminal (piped/redirected).
var (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
	Gray   = "\033[90m"
)

func init() {
	if !isTerminal(os.Stderr) {
		Reset = ""
		Bold = ""
		Red = ""
		Green = ""
		Yellow = ""
		Blue = ""
		Cyan = ""
		Gray = ""
	}
}

// isTerminal checks if the file is a terminal (TTY) using Stat.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	// If the file mode includes ModeCharDevice, it's a terminal
	return fi.Mode()&os.ModeCharDevice != 0
}
