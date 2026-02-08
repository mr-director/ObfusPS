package engine

import (
	mathrand "math/rand"
	"regexp"
)

type Options struct {
	InputFile   string
	OutputFile  string
	Level       int
	NoExec      bool
	UseStdin    bool
	UseStdout   bool
	Seed        int64
	Seeded      bool
	VarRename   bool
	MinFrag     int
	MinFragSet  bool
	MaxFrag     int
	MaxFragSet  bool
	Quiet       bool
	Pipeline    string
	StringDict  int
	StrEnc      string
	StrKeyHex   string
	NumEnc      bool
	IdenMode    string
	FormatMode  string
	CFOpaque    bool
	CFShuffle   bool
	DeadProb    int
	FragProfile string
	Profile     string
	Fuzz        int
	NoIntegrity bool   // level 5: disable integrity check (hash) to avoid empty output if file is re-saved
	LogFile     string // optional: debug log file (empty = disabled)
	// Pro / modular options
	Layers      string // Comma-separated: AST,Flow,Encoding,Runtime (overrides pipeline+level when set)
	Report      bool   // Emit obfuscation report after build
	DryRun      bool   // Analyze only, no transformation or output
	AntiReverse bool   // Inject anti-debug/sandbox checks (redteam/paranoid profiles)
	FlowSafeMode   bool   // When true (default): no CF/dead transforms on try-catch, trap, etc. redteam/paranoid or -flow-unsafe disable.
	Validate       bool   // After obfuscation: run original and obfuscated, compare outputs
	ValidateArgs   string // Optional args for -validate: e.g. "-Name x -Count 5"
	ValidateStderr  string // strict|ignore for -validate
	ValidateTimeout int    // seconds for -validate (0=30)
	ModuleAware     bool   // Protect Import-Module, dot-sourcing, exports
	ContextAware   bool   // Skip strenc/stringdict for strings in IEX, Add-Type, ScriptBlock::Create
	UseAST         bool   // Use native PowerShell AST for context-aware and module-aware (requires pwsh/powershell)
	// Smart / autonomous options
	Auto           bool   // Auto-detect best profile and level based on script analysis
	Recommend      bool   // Analyze script and print recommendations (no transformation)
	AutoRetry      bool   // When -validate fails, auto-retry with progressively safer settings
}

type Transform interface {
	Apply(ps string, ctx *Ctx) (string, error)
	Name() string
}

type Ctx struct {
	Rng       *mathrand.Rand
	Opts      *Options
	InputHash string
	Helpers   map[string]bool
	ASTResult *ASTResult // When -use-ast: result of native PowerShell AST parse; nil = use regex fallback
}

var (
	// reVar matches PowerShell variables including scoped variables ($script:var, $global:var, etc.)
	reVar         = regexp.MustCompile(`\$(?:(?:global|local|script|private|using):)?[A-Za-z_][A-Za-z0-9_]*`)
	reFuncHeader  = regexp.MustCompile(`(?i)\bfunction\s+([A-Za-z_][A-Za-z0-9_-]*)\s*\(`)
	reFuncNoParam = regexp.MustCompile(`(?i)\bfunction\s+([A-Za-z_][A-Za-z0-9_-]*)\s*{`)
	reParam       = regexp.MustCompile(`(?i)\$[A-Za-z_][A-Za-z0-9_]*`)
	reNum         = regexp.MustCompile(`\b\d+\b`)
	// reDQ and reSQ match single-line string literals only (no \r\n).
	// Multi-line strings are here-strings (@'...'@ and @"..."@) handled by findHereStringSpans.
	// Excluding newlines prevents "fake" matches between closing " of one string
	// and opening " of the next string on a different line.
	// The backtick escape `[^\r\n] covers ALL PS escapes: `$, `n, `t, `", ``, `0, etc.
	reDQ                  = regexp.MustCompile("\"(?:[^\"\r\n`]|`[^\r\n])*\"")
	reSQ                  = regexp.MustCompile("'(?:[^'\r\n]|'')*'")
	reExportModuleMember  = regexp.MustCompile(`(?i)Export-ModuleMember\s+-Function\s+([A-Za-z0-9_\-\s,]+?)(?:\s+-[A-Za-z]|\s*$|\r|\n)`)
)
