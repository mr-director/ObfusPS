package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Report holds obfuscation session data for reporting.
type Report struct {
	InputPath       string        `json:"inputPath"`
	OutputPath      string        `json:"outputPath"`
	Profile         string        `json:"profile"`
	Layers          []string      `json:"layers,omitempty"`
	Level           int           `json:"level"`
	Techniques      []string      `json:"techniques"`
	ComplexityScore int           `json:"complexityScore"`
	InputSize       int           `json:"inputSize"`
	OutputSize      int           `json:"outputSize"`
	FragmentCount   int           `json:"fragmentCount,omitempty"`
	Seed            int64         `json:"seed"`
	Warnings        []string      `json:"warnings,omitempty"`
	Duration        time.Duration `json:"duration,omitempty"`
	Entropy         float64       `json:"entropy,omitempty"`
	SizeRatio       float64       `json:"sizeRatio,omitempty"`
}

// ToJSON returns the report as indented JSON (for CI/CD integration).
func (r *Report) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// ComplexityScore computes a 0-100 score from techniques and metrics.
func (r *Report) ComputeComplexityScore(m Metrics) int {
	score := 0
	for _, t := range r.Techniques {
		switch strings.ToLower(t) {
		case "iden", "identifier":
			score += 5
		case "strenc", "string-encryption":
			score += 15
		case "stringdict":
			score += 8
		case "numenc":
			score += 5
		case "fmt", "format-jitter":
			score += 3
		case "cf-opaque", "cf-shuffle":
			score += 10
		case "deadcode":
			score += 7
		case "chars-join", "level1":
			score += 5
		case "base64", "level2", "level3":
			score += 10
		case "gzip", "level4":
			score += 20
		case "gzip-xor-frag", "level5":
			score += 30
		case "anti-reverse":
			score += 5
		default:
			score += 5
		}
		if score >= 100 {
			return 100
		}
	}
	// Entropy bonus
	if m.Entropy > 4.5 {
		score += 5
	}
	if score > 100 {
		score = 100
	}
	return score
}

// PrintReport writes the obfuscation report to stderr.
func PrintReport(r Report, m Metrics) {
	r.ComplexityScore = r.ComputeComplexityScore(m)
	r.Entropy = m.Entropy
	if r.InputSize > 0 {
		r.SizeRatio = float64(r.OutputSize) / float64(r.InputSize)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%s%s=== ObfusPS Report ===%s\n", Bold, Cyan, Reset)
	fmt.Fprintf(os.Stderr, "%sInput:%s    %s\n", Yellow, Reset, r.InputPath)
	fmt.Fprintf(os.Stderr, "%sOutput:%s   %s\n", Yellow, Reset, r.OutputPath)
	fmt.Fprintf(os.Stderr, "%sProfile:%s  %s%s%s\n", Yellow, Reset, Green, r.Profile, Reset)
	if len(r.Layers) > 0 {
		fmt.Fprintf(os.Stderr, "%sLayers:%s   %s\n", Yellow, Reset, strings.Join(r.Layers, ", "))
	}
	fmt.Fprintf(os.Stderr, "%sLevel:%s    %s%d%s\n", Yellow, Reset, Green, r.Level, Reset)
	fmt.Fprintf(os.Stderr, "%sTechniques:%s %s\n", Yellow, Reset, strings.Join(r.Techniques, ", "))
	fmt.Fprintf(os.Stderr, "%sComplexity score:%s %s%d%s/100\n", Yellow, Reset, Green, r.ComplexityScore, Reset)
	fmt.Fprintf(os.Stderr, "%sInput size:%s  %d bytes\n", Yellow, Reset, r.InputSize)
	fmt.Fprintf(os.Stderr, "%sOutput size:%s %d bytes", Yellow, Reset, r.OutputSize)
	if r.SizeRatio > 0 {
		fmt.Fprintf(os.Stderr, " %s(%.1fx)%s", Gray, r.SizeRatio, Reset)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "%sEntropy:%s   %.2f bits/symbol\n", Yellow, Reset, r.Entropy)
	if r.FragmentCount > 0 {
		fmt.Fprintf(os.Stderr, "%sFragments:%s %d\n", Yellow, Reset, r.FragmentCount)
	}
	fmt.Fprintf(os.Stderr, "%sSeed:%s %d\n", Yellow, Reset, r.Seed)
	if r.Duration > 0 {
		fmt.Fprintf(os.Stderr, "%sDuration:%s  %s\n", Yellow, Reset, r.Duration.Round(time.Millisecond))
	}
	if len(r.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "%sWarnings:%s\n", Red, Reset)
		for _, w := range r.Warnings {
			fmt.Fprintf(os.Stderr, "  - %s\n", w)
		}
	}
	fmt.Fprintf(os.Stderr, "%sArchitecture:%s Go engine never embeds PowerShell; all runtime behavior lives in generated stubs.\n", Gray, Reset)
	fmt.Fprintf(os.Stderr, "%sAST:%s Regex-based; native PowerShell AST is the only real hard gap (no symbol graph). See docs/ROADMAP.md.\n", Gray, Reset)
	fmt.Fprintf(os.Stderr, "%s%s======================%s\n", Bold, Cyan, Reset)
}
