package engine

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// Metrics holds objective measures on the generated script.
type Metrics struct {
	SizeBytes        int     // Size in bytes
	UniqueSymbols    int     // Number of unique runes
	Entropy          float64 // Approximate entropy (bits per symbol)
	AlnumRatio       float64 // Alphanumeric characters ratio / total (0-1)
	CompressionRatio float64 // output/input size ratio (>1 = larger)
	LineCount        int     // Number of lines in output
	InputSizeBytes   int     // Size of original input (for ratio computation)
}

// ComputeMetrics computes metrics on the generated payload.
func ComputeMetrics(payload string) Metrics {
	m := Metrics{SizeBytes: len(payload)}
	if m.SizeBytes == 0 {
		return m
	}
	freq := make(map[rune]int)
	alnum := 0
	for _, r := range payload {
		freq[r]++
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			alnum++
		}
	}
	m.UniqueSymbols = len(freq)
	m.AlnumRatio = float64(alnum) / float64(len(payload))
	m.LineCount = strings.Count(payload, "\n") + 1
	n := float64(len(payload))
	for _, c := range freq {
		if c <= 0 {
			continue
		}
		p := float64(c) / n
		m.Entropy -= p * math.Log2(p)
	}
	return m
}

// ComputeMetricsWithInput computes metrics with input size for compression ratio.
func ComputeMetricsWithInput(payload string, inputSize int) Metrics {
	m := ComputeMetrics(payload)
	m.InputSizeBytes = inputSize
	if inputSize > 0 {
		m.CompressionRatio = float64(m.SizeBytes) / float64(inputSize)
	}
	return m
}

// PrintMetrics prints metrics to stderr (if !quiet).
func PrintMetrics(m Metrics, quiet bool) {
	if quiet {
		return
	}
	line := fmt.Sprintf("%sMetrics:%s size=%s%d%s bytes | unique=%s%d%s | entropy=%.2f | alnum_ratio=%.2f",
		Cyan, Reset, Green, m.SizeBytes, Reset, Green, m.UniqueSymbols, Reset, m.Entropy, m.AlnumRatio)
	if m.CompressionRatio > 0 {
		line += fmt.Sprintf(" | ratio=%.1fx", m.CompressionRatio)
	}
	if m.LineCount > 0 {
		line += fmt.Sprintf(" | lines=%d", m.LineCount)
	}
	fmt.Fprintln(os.Stderr, line)
}
