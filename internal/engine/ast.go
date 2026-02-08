package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ASTResult holds the result of PowerShell AST parsing.
type ASTResult struct {
	ExecutableStrings [][]int  `json:"executableStrings"`
	ExportedFunctions []string `json:"exportedFunctions"`
	Error             string   `json:"error,omitempty"`
}

// RunASTParse invokes the PowerShell AST helper (or C# fallback on Windows) and returns parsed result.
// Order: 1) pwsh/powershell + ast-parse.ps1, 2) PSAstParser.exe on Windows, 3) nil+error (caller fallback to regex).
// Uses stdin for scripts >4KB to avoid Windows command-line limit (8191 chars).
func RunASTParse(scriptContent string) (*ASTResult, error) {
	// 1) PowerShell first
	if pwsh, err := findPowerShellForAST(); err == nil {
		helperPath, err := findASTHelperPath()
		if err == nil {
			result, err := runPowerShellAST(pwsh, helperPath, scriptContent)
			if err == nil {
				return result, nil
			}
		}
	}

	// 2) C# fallback on Windows (PSAstParser.exe)
	if runtime.GOOS == "windows" {
		if csPath, err := findPSAstParserPath(); err == nil {
			result, err := runPSAstParser(csPath, scriptContent)
			if err == nil {
				return result, nil
			}
		}
	}

	return nil, fmt.Errorf("PowerShell not found (pwsh or powershell required for -use-ast)")
}

const astTimeout = 30 * time.Second

func runPowerShellAST(pwsh, helperPath, script string) (*ASTResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), astTimeout)
	defer cancel()
	var cmd *exec.Cmd
	const maxArgLen = 4000 // Windows cmd line ~8191; use stdin for larger
	if len(script) > maxArgLen {
		cmd = exec.CommandContext(ctx, pwsh, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", helperPath)
		cmd.Stdin = strings.NewReader(script)
	} else {
		cmd = exec.CommandContext(ctx, pwsh, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", helperPath, "-InputScript", script)
	}
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("ast helper: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("ast helper: %w", err)
	}
	return parseASTOutput(out)
}

func runPSAstParser(exePath, script string) (*ASTResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), astTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exePath)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("PSAstParser: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("PSAstParser: %w", err)
	}
	return parseASTOutput(out)
}

func parseASTOutput(out []byte) (*ASTResult, error) {
	// Trim stray stderr (PowerShell profile can write to stdout)
	out = bytes.TrimSpace(out)
	// Take last line if multiple (JSON only)
	if idx := bytes.LastIndexByte(out, '\n'); idx >= 0 {
		last := bytes.TrimSpace(out[idx+1:])
		if len(last) > 0 && last[0] == '{' {
			out = last
		}
	}
	var result ASTResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("ast json parse: %w", err)
	}
	return &result, nil
}

func findPowerShellForAST() (string, error) {
	for _, name := range []string{"pwsh", "powershell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("PowerShell not found")
}

// findASTHelperPath returns the path to scripts/ast-parse.ps1.
// Searches: OBFUSPS_ROOT env, cwd, exe dir, cwd parent, exe parent.
func findASTHelperPath() (string, error) {
	var bases []string
	if root := os.Getenv("OBFUSPS_ROOT"); root != "" {
		bases = append(bases, root)
	}
	if cwd, err := os.Getwd(); err == nil {
		bases = append(bases, cwd, filepath.Dir(cwd), filepath.Join(filepath.Dir(cwd), ".."))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		bases = append(bases, exeDir, filepath.Join(exeDir, ".."))
	}
	seen := make(map[string]bool)
	for _, base := range bases {
		base = filepath.Clean(base)
		if base == "" || base == "." || seen[base] {
			continue
		}
		seen[base] = true
		p := filepath.Join(base, "scripts", "ast-parse.ps1")
		if pathExists(p) {
			abs, err := filepath.Abs(p)
			if err != nil {
				return "", fmt.Errorf("cannot resolve path %s: %w", p, err)
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("scripts/ast-parse.ps1 not found (set OBFUSPS_ROOT or run from project root)")
}

// findPSAstParserPath returns the path to PSAstParser.exe (Windows).
func findPSAstParserPath() (string, error) {
	var bases []string
	if root := os.Getenv("OBFUSPS_ROOT"); root != "" {
		bases = append(bases, root)
	}
	if cwd, err := os.Getwd(); err == nil {
		bases = append(bases, cwd, filepath.Join(cwd, "build", "PSAstParser"),
			filepath.Join(cwd, "scripts", "PSAstParser", "bin", "Release", "net8.0"),
			filepath.Join(filepath.Dir(cwd), "build", "PSAstParser"))
	}
	if exe, err := os.Executable(); err == nil {
		bases = append(bases, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "build", "PSAstParser"))
	}
	names := []string{"PSAstParser.exe", "PSAstParser"}
	for _, name := range names {
		for _, base := range bases {
			p := filepath.Join(base, name)
			if pathExists(p) {
				abs, err := filepath.Abs(p)
				if err != nil {
					continue
				}
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("PSAstParser.exe not found")
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
