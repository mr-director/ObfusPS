package engine

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ScriptFeatures holds the result of static analysis on a PowerShell script.
type ScriptFeatures struct {
	HasClasses          bool
	HasHereStrings      bool
	HasSplatting        bool
	HasDynamicInvoke    bool // Invoke-Expression, IEX, [scriptblock]::Create
	HasAddType          bool // Add-Type (inline C#/VB)
	HasDotNet           bool // [System.Something], .NET reflection
	HasWMI              bool // Get-WmiObject, Get-CimInstance
	HasCmdletBinding    bool // [CmdletBinding()] advanced functions
	HasModulePatterns   bool // Import-Module, Export-ModuleMember, .psm1
	HasClosures         bool // { $_ }, ForEach-Object { }, Where-Object { }
	HasCrypto           bool // SHA, AES, RSA, HMAC, crypto APIs
	HasANSI             bool // ANSI escape sequences, colors
	HasRegex            bool // -match, -replace, [regex]
	HasXML              bool // [xml], Select-Xml, XPath
	HasJSON             bool // ConvertTo-Json, ConvertFrom-Json
	HasBackgroundJobs   bool // Start-Job, Start-ThreadJob, runspaces
	HasFileIO           bool // Get-Content, Set-Content, [IO.File]
	HasRemoting         bool // Invoke-Command, Enter-PSSession, -ComputerName
	HasErrorHandling    bool // try/catch/finally, trap, $ErrorActionPreference
	HasEnums            bool // enum keyword
	HasBracedVars       bool // ${varname} syntax
	HasScriptBlocks     bool // [scriptblock], & { }
	HasFormatStrings    bool // -f operator, .NET format strings

	LineCount           int
	FunctionCount       int
	ClassCount          int
	StringCount         int
	Complexity          int    // 0-100 score
	RecommendedProfile  string
	RecommendedLevel    int
	Warnings            []string
	Suggestions         []string
}

// AnalyzeScript performs static analysis on a PowerShell script and returns
// detected features, complexity score, and smart recommendations.
func AnalyzeScript(ps string) *ScriptFeatures {
	f := &ScriptFeatures{}

	// Line count
	f.LineCount = strings.Count(ps, "\n") + 1

	// --- Feature detection ---
	lower := strings.ToLower(ps)

	// Classes & Enums
	reClass := regexp.MustCompile(`(?im)^\s*class\s+\w+`)
	f.ClassCount = len(reClass.FindAllString(ps, -1))
	f.HasClasses = f.ClassCount > 0
	f.HasEnums = regexp.MustCompile(`(?im)^\s*enum\s+\w+`).MatchString(ps)

	// Functions
	f.FunctionCount = len(reFuncHeader.FindAllString(ps, -1)) + len(reFuncNoParam.FindAllString(ps, -1))

	// Strings
	dqCount := len(reDQ.FindAllString(ps, -1))
	sqCount := len(reSQ.FindAllString(ps, -1))
	f.StringCount = dqCount + sqCount

	// Here-strings
	f.HasHereStrings = strings.Contains(ps, "@'") || strings.Contains(ps, "@\"")

	// Splatting
	reSplat := regexp.MustCompile(`@[A-Za-z_]\w+\b`)
	f.HasSplatting = reSplat.MatchString(ps) && strings.Contains(lower, "splat")
	// Also check for @{ } hash passed with @var syntax
	if !f.HasSplatting {
		reSplatUse := regexp.MustCompile(`\w+\s+@[A-Za-z_]\w+`)
		f.HasSplatting = reSplatUse.MatchString(ps)
	}

	// Dynamic invocation
	f.HasDynamicInvoke = strings.Contains(lower, "invoke-expression") ||
		regexp.MustCompile(`(?i)\bIEX\b`).MatchString(ps) ||
		strings.Contains(lower, "scriptblock]::create") ||
		strings.Contains(lower, ".invoke(")

	// Add-Type
	f.HasAddType = strings.Contains(lower, "add-type")

	// .NET
	f.HasDotNet = regexp.MustCompile(`\[System\.\w+`).MatchString(ps) ||
		regexp.MustCompile(`\[(?:IO|Net|Text|Security|Collections|Reflection)\.\w+`).MatchString(ps)

	// WMI/CIM
	f.HasWMI = strings.Contains(lower, "get-wmiobject") ||
		strings.Contains(lower, "get-ciminstance") ||
		strings.Contains(lower, "invoke-cimmethod")

	// CmdletBinding
	f.HasCmdletBinding = strings.Contains(lower, "cmdletbinding")

	// Module patterns
	f.HasModulePatterns = strings.Contains(lower, "import-module") ||
		strings.Contains(lower, "export-modulemember") ||
		strings.Contains(lower, ".psm1")

	// Closures / pipeline blocks
	f.HasClosures = strings.Contains(lower, "foreach-object") ||
		strings.Contains(lower, "where-object") ||
		strings.Contains(ps, "{ $_ }") ||
		strings.Contains(ps, "{$_}")

	// Crypto
	f.HasCrypto = strings.Contains(lower, "sha256") ||
		strings.Contains(lower, "sha1") ||
		strings.Contains(lower, "aes") ||
		strings.Contains(lower, "hmac") ||
		strings.Contains(lower, "cryptography") ||
		strings.Contains(lower, "rsacryptoserviceprovider")

	// ANSI
	f.HasANSI = strings.Contains(ps, "\x1b[") || strings.Contains(ps, "`e[") ||
		strings.Contains(ps, "$([char]27)")

	// Regex
	f.HasRegex = strings.Contains(lower, "-match") ||
		strings.Contains(lower, "-replace") ||
		strings.Contains(lower, "[regex]")

	// XML
	f.HasXML = strings.Contains(lower, "[xml]") ||
		strings.Contains(lower, "select-xml") ||
		strings.Contains(lower, "system.xml")

	// JSON
	f.HasJSON = strings.Contains(lower, "convertto-json") ||
		strings.Contains(lower, "convertfrom-json")

	// Background jobs / runspaces
	f.HasBackgroundJobs = strings.Contains(lower, "start-job") ||
		strings.Contains(lower, "start-threadjob") ||
		strings.Contains(lower, "runspacefactory") ||
		strings.Contains(lower, "runspacepool")

	// File I/O
	f.HasFileIO = strings.Contains(lower, "get-content") ||
		strings.Contains(lower, "set-content") ||
		strings.Contains(lower, "[io.file]") ||
		strings.Contains(lower, "out-file")

	// Remoting
	f.HasRemoting = strings.Contains(lower, "invoke-command") &&
		strings.Contains(lower, "-computername")

	// Error handling
	f.HasErrorHandling = strings.Contains(lower, "try") && strings.Contains(lower, "catch") ||
		strings.Contains(lower, "trap {") ||
		strings.Contains(lower, "$erroractionpreference")

	// Braced variables
	f.HasBracedVars = regexp.MustCompile(`\$\{[^}]+\}`).MatchString(ps)

	// Script blocks
	f.HasScriptBlocks = strings.Contains(lower, "[scriptblock]") ||
		regexp.MustCompile(`&\s*\{`).MatchString(ps)

	// Format strings
	f.HasFormatStrings = regexp.MustCompile(`-f\s+['"]`).MatchString(ps) ||
		strings.Contains(ps, "'{0}'") ||
		strings.Contains(ps, "\"{0}\"")

	// --- Complexity score ---
	score := 0
	if f.LineCount > 50 {
		score += 10
	}
	if f.LineCount > 200 {
		score += 10
	}
	if f.LineCount > 500 {
		score += 10
	}
	if f.FunctionCount > 3 {
		score += 10
	}
	if f.FunctionCount > 10 {
		score += 5
	}
	if f.HasClasses {
		score += 15
	}
	if f.HasEnums {
		score += 5
	}
	if f.HasHereStrings {
		score += 10
	}
	if f.HasSplatting {
		score += 5
	}
	if f.HasDynamicInvoke {
		score += 15
	}
	if f.HasAddType {
		score += 15
	}
	if f.HasDotNet {
		score += 5
	}
	if f.HasCrypto {
		score += 5
	}
	if f.HasCmdletBinding {
		score += 5
	}
	if f.HasModulePatterns {
		score += 10
	}
	if f.HasBracedVars {
		score += 3
	}
	if f.HasFormatStrings {
		score += 3
	}
	if f.HasScriptBlocks {
		score += 5
	}
	if f.HasBackgroundJobs {
		score += 10
	}
	if f.HasRemoting {
		score += 10
	}
	if f.HasXML {
		score += 5
	}
	if score > 100 {
		score = 100
	}
	f.Complexity = score

	// --- Smart recommendations ---
	f.computeRecommendations()

	return f
}

func (f *ScriptFeatures) computeRecommendations() {
	// Profile recommendation based on complexity
	switch {
	case f.Complexity <= 15:
		f.RecommendedProfile = "balanced"
		f.RecommendedLevel = 5
	case f.Complexity <= 35:
		f.RecommendedProfile = "heavy"
		f.RecommendedLevel = 5
	case f.Complexity <= 60:
		f.RecommendedProfile = "heavy"
		f.RecommendedLevel = 4
	default:
		f.RecommendedProfile = "balanced"
		f.RecommendedLevel = 4
	}

	// Warnings
	if f.HasDynamicInvoke {
		f.Warnings = append(f.Warnings, "Script uses Invoke-Expression / IEX — strings passed to IEX must not be encrypted")
		f.Suggestions = append(f.Suggestions, "Use -context-aware -use-ast to protect IEX strings")
	}
	if f.HasAddType {
		f.Warnings = append(f.Warnings, "Script uses Add-Type (inline C#/VB) — C# code must not be modified by string transforms")
		f.Suggestions = append(f.Suggestions, "Use -context-aware -use-ast to protect Add-Type blocks")
	}
	if f.HasClasses {
		f.Warnings = append(f.Warnings, fmt.Sprintf("Script defines %d class(es) — class properties protected from renaming", f.ClassCount))
	}
	if f.HasHereStrings {
		f.Warnings = append(f.Warnings, "Script uses here-strings (@'...'@ / @\"...\"@) — content preserved by all transforms")
	}
	if f.HasSplatting {
		f.Warnings = append(f.Warnings, "Script uses splatting (@var) — @-references kept in sync with $-variables")
	}
	if f.HasModulePatterns {
		f.Warnings = append(f.Warnings, "Script uses Import-Module / Export-ModuleMember — exported names must be preserved")
		f.Suggestions = append(f.Suggestions, "Use -module-aware to protect exported function names")
	}
	if f.HasBracedVars {
		f.Warnings = append(f.Warnings, "Script uses ${braced} variable syntax — kept in sync with $-variable renames")
	}
	if f.HasBackgroundJobs {
		f.Warnings = append(f.Warnings, "Script uses background jobs / runspaces — script blocks passed to jobs may need protection")
	}
	if f.HasRemoting {
		f.Warnings = append(f.Warnings, "Script uses PS Remoting — Invoke-Command script blocks execute on remote machines")
	}
	if f.HasCmdletBinding {
		f.Suggestions = append(f.Suggestions, "Advanced functions detected — parameter attribute arguments auto-protected")
	}
	if f.Complexity >= 60 {
		f.Suggestions = append(f.Suggestions, "High complexity script — use -validate to verify output correctness")
	}

	// Override: if dynamic invoke or Add-Type, recommend context-aware
	if f.HasDynamicInvoke || f.HasAddType {
		if f.RecommendedProfile == "balanced" {
			f.RecommendedProfile = "heavy"
		}
	}
}

// PrintAnalysis prints the script analysis to stderr.
func PrintAnalysis(f *ScriptFeatures, quiet bool) {
	if quiet {
		return
	}
	fmt.Fprintf(os.Stderr, "\n%s╔══ Script Analysis ═══════════════════════════════════════╗%s\n", Cyan, Reset)
	fmt.Fprintf(os.Stderr, "%s║%s  Lines: %-6d  Functions: %-4d  Classes: %-4d          %s║%s\n",
		Cyan, Reset, f.LineCount, f.FunctionCount, f.ClassCount, Cyan, Reset)
	fmt.Fprintf(os.Stderr, "%s║%s  Strings: %-5d  Complexity: %d/100                      %s║%s\n",
		Cyan, Reset, f.StringCount, f.Complexity, Cyan, Reset)

	var features []string
	if f.HasClasses {
		features = append(features, "Classes")
	}
	if f.HasEnums {
		features = append(features, "Enums")
	}
	if f.HasHereStrings {
		features = append(features, "Here-Strings")
	}
	if f.HasSplatting {
		features = append(features, "Splatting")
	}
	if f.HasDynamicInvoke {
		features = append(features, "IEX/Dynamic")
	}
	if f.HasAddType {
		features = append(features, "Add-Type")
	}
	if f.HasDotNet {
		features = append(features, ".NET")
	}
	if f.HasWMI {
		features = append(features, "WMI/CIM")
	}
	if f.HasCmdletBinding {
		features = append(features, "CmdletBinding")
	}
	if f.HasCrypto {
		features = append(features, "Crypto")
	}
	if f.HasXML {
		features = append(features, "XML")
	}
	if f.HasJSON {
		features = append(features, "JSON")
	}
	if f.HasBackgroundJobs {
		features = append(features, "Jobs/Runspaces")
	}
	if f.HasFileIO {
		features = append(features, "File-IO")
	}
	if f.HasRegex {
		features = append(features, "Regex")
	}
	if f.HasANSI {
		features = append(features, "ANSI-Colors")
	}
	if f.HasModulePatterns {
		features = append(features, "Module")
	}
	if f.HasErrorHandling {
		features = append(features, "ErrorHandling")
	}
	if f.HasBracedVars {
		features = append(features, "BracedVars")
	}
	if f.HasScriptBlocks {
		features = append(features, "ScriptBlocks")
	}
	if f.HasFormatStrings {
		features = append(features, "FormatStrings")
	}

	featLine := strings.Join(features, ", ")
	if len(featLine) > 56 {
		// Split into multiple lines
		mid := len(features) / 2
		line1 := strings.Join(features[:mid], ", ")
		line2 := strings.Join(features[mid:], ", ")
		fmt.Fprintf(os.Stderr, "%s║%s  Features: %-48s%s║%s\n", Cyan, Reset, line1, Cyan, Reset)
		fmt.Fprintf(os.Stderr, "%s║%s            %-48s%s║%s\n", Cyan, Reset, line2, Cyan, Reset)
	} else if len(features) > 0 {
		fmt.Fprintf(os.Stderr, "%s║%s  Features: %-48s%s║%s\n", Cyan, Reset, featLine, Cyan, Reset)
	}

	fmt.Fprintf(os.Stderr, "%s╠══ Recommendation ════════════════════════════════════════╣%s\n", Cyan, Reset)
	fmt.Fprintf(os.Stderr, "%s║%s  %s→ Profile: %-10s  Level: %d%s                        %s║%s\n",
		Cyan, Reset, Green, f.RecommendedProfile, f.RecommendedLevel, Reset, Cyan, Reset)

	if f.HasDynamicInvoke || f.HasAddType {
		fmt.Fprintf(os.Stderr, "%s║%s  %s→ Auto-enabling: -context-aware -use-ast%s              %s║%s\n",
			Cyan, Reset, Green, Reset, Cyan, Reset)
	}
	if f.HasModulePatterns {
		fmt.Fprintf(os.Stderr, "%s║%s  %s→ Auto-enabling: -module-aware%s                         %s║%s\n",
			Cyan, Reset, Green, Reset, Cyan, Reset)
	}

	if len(f.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "%s╠══ Warnings ══════════════════════════════════════════════╣%s\n", Yellow, Reset)
		for _, w := range f.Warnings {
			// Truncate long warnings
			if len(w) > 57 {
				w = w[:54] + "..."
			}
			fmt.Fprintf(os.Stderr, "%s║%s  %s⚠ %s%s  %s║%s\n", Yellow, Reset, Yellow, w, Reset, Yellow, Reset)
		}
	}
	fmt.Fprintf(os.Stderr, "%s╚═════════════════════════════════════════════════════════╝%s\n\n", Cyan, Reset)
}
