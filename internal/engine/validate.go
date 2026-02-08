package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runValidate executes original and obfuscated scripts and compares stdout, stderr, exit code.
func runValidate(opts Options) error {
	if opts.UseStdin || opts.InputFile == "" {
		return fmt.Errorf("-validate requires -i (file input)")
	}
	if opts.UseStdout || opts.OutputFile == "" {
		return fmt.Errorf("-validate requires -o (output file)")
	}
	pwsh, err := findPowerShell()
	if err != nil {
		return err
	}
	args := buildValidateArgs(opts.ValidateArgs)
	timeout := time.Duration(opts.ValidateTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ignoreStderr := strings.EqualFold(strings.TrimSpace(opts.ValidateStderr), "ignore")

	origOut, origErr, origCode, err := runScript(pwsh, opts.InputFile, args, timeout)
	if err != nil {
		return fmt.Errorf("original script: %w", err)
	}

	obfOut, obfErr, obfCode, err := runScript(pwsh, opts.OutputFile, args, timeout)
	if err != nil {
		return fmt.Errorf("obfuscated script: %w", err)
	}

	stderrMatch := ignoreStderr || bytes.Equal(origErr, obfErr)
	ok := origCode == obfCode && bytes.Equal(origOut, obfOut) && stderrMatch
	if !opts.Quiet {
		if ok {
			fmt.Fprintf(os.Stderr, "%sValidate:%s PASS (exit %d, stdout/stderr match)\n", Green, Reset, origCode)
		} else {
			fmt.Fprintf(os.Stderr, "%sValidate:%s FAIL\n", Red, Reset)
			if origCode != obfCode {
				fmt.Fprintf(os.Stderr, "  exit: original=%d obfuscated=%d\n", origCode, obfCode)
			}
			if !bytes.Equal(origOut, obfOut) {
				fmt.Fprintf(os.Stderr, "  stdout differs (orig %d bytes, obf %d bytes)\n", len(origOut), len(obfOut))
			}
			if !ignoreStderr && !bytes.Equal(origErr, obfErr) {
				fmt.Fprintf(os.Stderr, "  stderr differs (orig %d bytes, obf %d bytes)\n", len(origErr), len(obfErr))
			}
		}
	}
	if !ok {
		return fmt.Errorf("validate failed: output or exit code differs")
	}
	return nil
}

func findPowerShell() (string, error) {
	for _, name := range []string{"pwsh", "powershell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("PowerShell not found (pwsh or powershell)")
}

func buildValidateArgs(s string) []string {
	if s == "" {
		return nil
	}
	// Parse respecting quoted strings: -Name "foo bar" -> ["-Name", "foo bar"]
	var out []string
	var buf strings.Builder
	inQuote := false
	var quote rune
	for _, r := range s {
		switch {
		case r == '"' || r == '\'':
			if !inQuote {
				inQuote = true
				quote = r
			} else if r == quote {
				inQuote = false
				out = append(out, buf.String())
				buf.Reset()
			} else {
				// Mismatched quote inside quoted string — treat as literal character
				buf.WriteRune(r)
			}
		case inQuote:
			buf.WriteRune(r)
		case r == ' ' || r == '\t':
			if buf.Len() > 0 {
				out = append(out, buf.String())
				buf.Reset()
			}
		default:
			buf.WriteRune(r)
		}
	}
	// If quote was never closed, flush remaining buffer as-is (graceful handling)
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

func runScript(pwsh, scriptPath string, scriptArgs []string, timeout time.Duration) (stdout, stderr []byte, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create a temporary wrapper that forces UTF-8 encoding before invoking the
	// target script.  The obfuscator embeds the same prefix in every obfuscated
	// payload, so the original script must run under the same encoding for a
	// fair byte-level comparison.
	absPath, pathErr := filepath.Abs(scriptPath)
	if pathErr != nil {
		absPath = scriptPath
	}
	escaped := strings.ReplaceAll(absPath, "'", "''")
	wrapper := fmt.Sprintf(
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8\n$OutputEncoding=[Text.Encoding]::UTF8\n& '%s' @args",
		escaped,
	)
	tmp, tmpErr := os.CreateTemp("", "obfusps-validate-*.ps1")
	if tmpErr != nil {
		return nil, nil, -1, tmpErr
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(wrapper)
	tmp.Close()

	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", tmp.Name()}
	args = append(args, scriptArgs...)
	cmd := exec.CommandContext(ctx, pwsh, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return outBuf.Bytes(), errBuf.Bytes(), exitErr.ExitCode(), nil
		}
		return nil, nil, -1, runErr
	}
	return outBuf.Bytes(), errBuf.Bytes(), 0, nil
}
