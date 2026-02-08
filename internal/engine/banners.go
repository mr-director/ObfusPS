package engine

import (
	"fmt"
	"runtime"
	"strings"
)

const (
	version = "1.2.0"
	author  = "BenzoXdev"
)

// bannerColor is the colored banner for CLI output.
var bannerColor = Cyan + "ObfusPS" + Reset + " | v." + version + " | " + Gray + "https://github.com/BenzoXdev/ObfusPS" + Reset

// PrintBanner prints the banner (for interactive mode).
func PrintBanner() {
	fmt.Print(bannerColor)
}

// Version returns the version string.
func Version() string {
	return version
}

// VersionFull returns version with Go and platform info.
func VersionFull() string {
	return fmt.Sprintf("ObfusPS v%s (%s/%s, %s)", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// ErrorHint returns a helpful hint for common errors.
func ErrorHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "file not found"):
		return "Check the path with -i. Use absolute paths or run from the project directory."
	case strings.Contains(msg, "not valid UTF-8"):
		return "Re-save the file as UTF-8 (with or without BOM) in your editor."
	case strings.Contains(msg, "missing -i") || strings.Contains(msg, "missing -stdin"):
		return "Specify input: obfusps -i script.ps1 -o out.ps1 -level 5"
	case strings.Contains(msg, "missing -strkey"):
		return "Provide a hex key: -strkey 00112233445566778899aabbccddeeff"
	case strings.Contains(msg, "PowerShell not found"):
		return "Install PowerShell 7 (pwsh) or ensure powershell is in PATH for -use-ast."
	case strings.Contains(msg, "file is empty"):
		return "The input file has no content. Check the path and file."
	case strings.Contains(msg, "validate failed"):
		return "Original and obfuscated scripts produce different output. Try -profile safe or add path fallback (see README §12)."
	case strings.Contains(msg, "too large"):
		return "The input file exceeds the safety limit. Split large scripts or increase the limit."
	case strings.Contains(msg, "encryption key is empty"):
		return "Use -strkey to provide a hex encryption key, or remove -strenc from the pipeline."
	}
	return ""
}
