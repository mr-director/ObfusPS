package engine

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// maxInputSize is a safety limit to prevent memory exhaustion (100 MB).
const maxInputSize = 100 * 1024 * 1024

// utf8BOM is the UTF-8 Byte Order Mark (EF BB BF).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripBOM removes the UTF-8 BOM from the beginning of data if present.
// The BOM must NOT be included in the payload — it would corrupt the first
// token when the obfuscated script decodes and executes the inner script
// via [scriptblock]::Create().  The output file gets its own BOM header.
func stripBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}

func readAllInput(opts Options) ([]byte, error) {
	if opts.UseStdin {
		data, err := io.ReadAll(io.LimitReader(bufio.NewReader(os.Stdin), maxInputSize+1))
		if err != nil {
			return nil, fmt.Errorf("stdin: %w", err)
		}
		if len(data) > maxInputSize {
			return nil, fmt.Errorf("input too large (>%d bytes, safety limit)", maxInputSize)
		}
		return stripBOM(data), nil
	}
	fi, err := os.Stat(opts.InputFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", opts.InputFile)
		}
		return nil, fmt.Errorf("reading input: %w", err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("input is a directory, not a file: %s", opts.InputFile)
	}
	if fi.Size() > maxInputSize {
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", fi.Size(), maxInputSize)
	}
	data, err := os.ReadFile(opts.InputFile)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	return stripBOM(data), nil
}

// validateUTF8 checks that data is valid UTF-8 (PowerShell expects text).
func validateUTF8(data []byte) error {
	if len(data) == 0 {
		return errors.New("file is empty")
	}
	if !utf8.Valid(data) {
		return errors.New("file is not valid UTF-8 — save it as UTF-8 (with or without BOM)")
	}
	return nil
}

func fuzzOutName(base string, i int) string {
	if base == "" {
		return fmt.Sprintf("obfuscated.v%d.ps1", i)
	}
	baseLower := strings.ToLower(base)
	if strings.HasSuffix(baseLower, ".ps1") {
		return base[:len(base)-4] + fmt.Sprintf(".v%d.ps1", i)
	}
	return base + fmt.Sprintf(".v%d.ps1", i)
}

func requireInOut(opts Options) error {
	if !opts.UseStdin && opts.InputFile == "" && !opts.DryRun {
		return errors.New("missing -i or -stdin (use -i <inputFile> or pipe script to stdin)")
	}
	if !opts.UseStdout && opts.OutputFile == "" && !opts.DryRun {
		return errors.New("missing -o or -stdout (use -dry-run for analysis only)")
	}
	return nil
}
