package engine

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func ParseFlags() (Options, bool) {
	opts := Options{}
	flag.StringVar(&opts.InputFile, "i", "", "PowerShell script input file (use -stdin).")
	flag.StringVar(&opts.OutputFile, "o", "obfuscated.ps1", "Output file (use -stdout).")
	flag.IntVar(&opts.Level, "level", 1, "Obfuscation level (1..5).")
	flag.BoolVar(&opts.NoExec, "noexec", false, "Emit only payload without Invoke-Expression.")
	flag.BoolVar(&opts.UseStdin, "stdin", false, "Read script from STDIN.")
	flag.BoolVar(&opts.UseStdout, "stdout", false, "Write result to STDOUT.")
	flag.BoolVar(&opts.VarRename, "varrename", false, "Deprecated: kept for backward-compatibility. Use -iden obf.")
	flag.IntVar(&opts.MinFrag, "minfrag", 10, "Minimum fragment size (level 5).")
	flag.IntVar(&opts.MaxFrag, "maxfrag", 20, "Maximum fragment size (level 5).")
	flag.BoolVar(&opts.Quiet, "q", false, "Quiet mode (no banner).")
	flag.StringVar(&opts.Pipeline, "pipeline", "", "Comma-separated transforms: iden,strenc,stringdict,numenc,fmt,cf,dead")
	flag.IntVar(&opts.StringDict, "stringdict", 0, "String tokenization percentage (0..100).")
	flag.StringVar(&opts.StrEnc, "strenc", "off", "String encryption: off|xor|rc4.")
	flag.StringVar(&opts.StrKeyHex, "strkey", "", "Hex key for -strenc.")
	flag.BoolVar(&opts.NumEnc, "numenc", false, "Enable number encoding.")
	flag.StringVar(&opts.IdenMode, "iden", "keep", "Identifier morphing: obf|keep.")
	flag.StringVar(&opts.FormatMode, "fmt", "off", "Format jitter: off|jitter.")
	flag.BoolVar(&opts.CFOpaque, "cf-opaque", false, "Enable opaque predicate wrapper.")
	flag.BoolVar(&opts.CFShuffle, "cf-shuffle", false, "Shuffle function blocks.")
	flag.IntVar(&opts.DeadProb, "deadcode", 0, "Dead-code injection probability (0..100).")
	flag.StringVar(&opts.FragProfile, "frag", "", "Fragmentation profile: profile=tight|medium|loose|pro.")
	flag.StringVar(&opts.Profile, "profile", "", "Preset: safe|light|balanced|heavy|stealth|paranoid|redteam|blueteam|size|dev.")
	flag.IntVar(&opts.Fuzz, "fuzz", 0, "Generate N fuzzed variants (unique seeds).")
	flag.BoolVar(&opts.NoIntegrity, "no-integrity", true, "Level 5: disable integrity check (default=true for reliability). Use -no-integrity=false to enable.")
	flag.StringVar(&opts.LogFile, "log", "", "Write debug log to file (optional, disabled by default).")
	flag.StringVar(&opts.Layers, "layers", "", "Comma-separated: AST,Flow,Encoding,Runtime (overrides -pipeline and -level).")
	flag.BoolVar(&opts.Report, "report", false, "Emit obfuscation report after build.")
	flag.BoolVar(&opts.DryRun, "dry-run", false, "Analyze only, no transformation or output.")
	flag.BoolVar(&opts.AntiReverse, "anti-reverse", false, "Inject anti-debug/sandbox checks.")
	flag.BoolVar(&opts.Validate, "validate", false, "After obfuscation: run original and obfuscated, compare outputs.")
	flag.StringVar(&opts.ValidateArgs, "validate-args", "", "Optional args for -validate, e.g. \"-Name x -Count 5\".")
	flag.StringVar(&opts.ValidateStderr, "validate-stderr", "strict", "strict: compare stderr; ignore: compare stdout+exit only.")
	flag.IntVar(&opts.ValidateTimeout, "validate-timeout", 30, "Seconds timeout for -validate script execution.")
	flag.BoolVar(&opts.ModuleAware, "module-aware", false, "Protect Import-Module, dot-sourcing, exports.")
	flag.BoolVar(&opts.ContextAware, "context-aware", false, "Skip strenc/stringdict for strings in IEX, Add-Type, ScriptBlock::Create.")
	flag.BoolVar(&opts.UseAST, "use-ast", false, "Use native PowerShell AST for context-aware and module-aware (requires pwsh/powershell, scripts/ast-parse.ps1).")
	flag.BoolVar(&opts.Auto, "auto", false, "Smart mode: auto-detect best profile, level, and flags based on script analysis.")
	flag.BoolVar(&opts.Recommend, "recommend", false, "Analyze script and print recommendations without obfuscating.")
	flag.BoolVar(&opts.AutoRetry, "auto-retry", false, "When -validate fails, auto-retry with progressively safer settings.")
	var flowUnsafe bool
	flag.BoolVar(&flowUnsafe, "flow-unsafe", false, "Disable FlowSafeMode (allow CF/dead transforms; redteam/paranoid only).")
	var showHelp bool
	flag.BoolVar(&showHelp, "h", false, "Show help.")
	flag.BoolVar(&showHelp, "help", false, "Show help.")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Show version and exit.")
	var showDocs, showExamples bool
	flag.BoolVar(&showDocs, "docs", false, "Show best practices, recommended commands, and doc links.")
	flag.BoolVar(&showExamples, "examples", false, "Same as -docs: golden rules and copy-paste ready commands.")
	var seed int64
	flag.Int64Var(&seed, "seed", 0, "RNG seed (0=random/non-deterministic). Set N for reproducible build.")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n  obfusps -i input.ps1 -o out.ps1 -level 1..5 [options]\n")
		fmt.Fprintf(os.Stderr, "  obfusps build -i input.ps1 -layers AST,Flow,Encoding,Runtime -profile stealth -o out.ps1\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nArchitecture: Go engine never embeds PowerShell; all runtime behavior lives in generated stubs.\n")
		fmt.Fprintf(os.Stderr, "AST: Regex-based; native PowerShell AST is the only real hard gap. See docs/ROADMAP.md.\n")
		fmt.Fprintf(os.Stderr, "Docs: docs/BEST_PRACTICES.md | docs/DOCUMENTATION.md | docs/ROADMAP.md\n")
		fmt.Fprintf(os.Stderr, "Run -docs or -examples for golden rules and recommended commands.\n")
	}
	flag.Parse()
	if showVersion {
		fmt.Fprintln(os.Stderr, VersionFull())
		return Options{}, true
	}
	if showHelp {
		flag.Usage()
		return Options{}, true // help shown, caller should exit
	}
	if showDocs || showExamples {
		printDocsSummary()
		return Options{}, true // docs/examples shown, caller should exit
	}
	opts.FlowSafeMode = !flowUnsafe
	opts.Seeded = flag.Lookup("seed").Value.String() != "0"
	if opts.Seeded {
		opts.Seed = seed
	}
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "minfrag" {
			opts.MinFragSet = true
		}
		if f.Name == "maxfrag" {
			opts.MaxFragSet = true
		}
	})
	return opts, false
}

func applyProfileDefaults(opts *Options) error {
	switch strings.ToLower(opts.Profile) {
	case "":
	case "light":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,stringdict,numenc,frag"
		}
		if !opts.Seeded {
			opts.Seeded = true
			opts.Seed = 1337
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=tight"
		}
	case "balanced":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,stringdict,strenc,numenc,fmt,cf,dead,frag"
		}
		if !opts.Seeded {
			opts.Seeded = true
			opts.Seed = 424242
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=medium"
		}
		if opts.StrEnc == "off" {
			opts.StrEnc = "xor"
			if opts.StrKeyHex == "" {
				opts.StrKeyHex = "a1b2c3d4"
			}
		}
		if opts.StringDict == 0 {
			opts.StringDict = 30
		}
		if opts.DeadProb == 0 {
			opts.DeadProb = 10
		}
	case "heavy":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,stringdict,strenc,numenc,fmt,cf,dead,frag"
		}
		if opts.IdenMode == "keep" {
			opts.IdenMode = "obf"
		}
		if !opts.Seeded {
			opts.Seeded = true
			opts.Seed = 987654321
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=loose"
		}
		if opts.StrEnc == "off" {
			opts.StrEnc = "rc4"
			if opts.StrKeyHex == "" {
				opts.StrKeyHex = "00112233445566778899aabbccddeeff"
			}
		}
		if opts.StringDict == 0 {
			opts.StringDict = 50
		}
		if opts.DeadProb == 0 {
			opts.DeadProb = 25
		}
	case "redteam":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,stringdict,strenc,numenc,fmt,cf,dead,frag"
		}
		if opts.IdenMode == "keep" {
			opts.IdenMode = "obf"
		}
		if !opts.Seeded {
			opts.Seeded = false
			opts.Seed = 0
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=pro"
		}
		if opts.StrEnc == "off" {
			opts.StrEnc = "rc4"
			if opts.StrKeyHex == "" {
				opts.StrKeyHex = "00112233445566778899aabbccddeeff"
			}
		}
		if opts.StringDict == 0 {
			opts.StringDict = 50
		}
		if opts.DeadProb == 0 {
			opts.DeadProb = 25
		}
		opts.AntiReverse = true
		opts.FlowSafeMode = false
		opts.Level = 5
		opts.NoIntegrity = true
	case "blueteam":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,stringdict,strenc,numenc,fmt,cf,dead,frag"
		}
		if !opts.Seeded {
			opts.Seeded = true
			opts.Seed = 424242
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=medium"
		}
		if opts.StrEnc == "off" {
			opts.StrEnc = "xor"
			if opts.StrKeyHex == "" {
				opts.StrKeyHex = "a1b2c3d4"
			}
		}
		if opts.StringDict == 0 {
			opts.StringDict = 30
		}
		if opts.DeadProb == 0 {
			opts.DeadProb = 10
		}
		opts.AntiReverse = false
		opts.Level = 5
	case "size":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,frag"
		}
		if !opts.Seeded {
			opts.Seeded = true
			opts.Seed = 1337
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=tight"
		}
		opts.Level = 4
		opts.StringDict = 0
		opts.DeadProb = 0
		opts.FormatMode = "off"
	case "safe":
		// Preserve behavior: encoding only, no var/function renaming, no flow transforms.
		if opts.Pipeline == "" {
			opts.Pipeline = "iden"
		}
		opts.IdenMode = "keep" // no renaming => results identical to original
		if !opts.Seeded {
			opts.Seeded = true
			opts.Seed = 42
		}
		opts.Level = 3
		opts.StrEnc = "off"
		opts.StringDict = 0
		opts.DeadProb = 0
		opts.NumEnc = false
		opts.FormatMode = "off"
		opts.CFOpaque = false
		opts.CFShuffle = false
		opts.FlowSafeMode = true
	case "dev":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden"
		}
		if !opts.Seeded {
			opts.Seeded = true
			opts.Seed = 42
		}
		opts.Level = 2
		opts.StrEnc = "off"
		opts.StringDict = 0
		opts.DeadProb = 0
		opts.NumEnc = false
		opts.FormatMode = "off"
		opts.CFOpaque = false
		opts.CFShuffle = false
	case "stealth":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,stringdict,strenc,numenc,fmt,cf,dead,frag"
		}
		if opts.IdenMode == "keep" {
			opts.IdenMode = "obf"
		}
		if !opts.Seeded {
			opts.Seeded = false
			opts.Seed = 0
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=pro"
		}
		if opts.StrEnc == "off" {
			opts.StrEnc = "rc4"
			if opts.StrKeyHex == "" {
				opts.StrKeyHex = "00112233445566778899aabbccddeeff"
			}
		}
		if opts.StringDict == 0 {
			opts.StringDict = 40
		}
		if opts.DeadProb == 0 {
			opts.DeadProb = 15
		}
		opts.Level = 5
		opts.NoIntegrity = true
	case "paranoid":
		if opts.Pipeline == "" {
			opts.Pipeline = "iden,stringdict,strenc,numenc,fmt,cf,dead,frag"
		}
		if opts.IdenMode == "keep" {
			opts.IdenMode = "obf"
		}
		if !opts.Seeded {
			opts.Seeded = false
			opts.Seed = 0
		}
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=pro"
		}
		if opts.StrEnc == "off" {
			opts.StrEnc = "rc4"
			if opts.StrKeyHex == "" {
				opts.StrKeyHex = "00112233445566778899aabbccddeeff"
			}
		}
		if opts.StringDict == 0 {
			opts.StringDict = 60
		}
		if opts.DeadProb == 0 {
			opts.DeadProb = 30
		}
		opts.AntiReverse = true
		opts.FlowSafeMode = false
		opts.Level = 5
		opts.NoIntegrity = true
	default:
		return fmt.Errorf("invalid -profile: %s (use: safe|light|balanced|heavy|stealth|paranoid|redteam|blueteam|size|dev)", opts.Profile)
	}
	return nil
}

// applyLayers maps -layers (AST,Flow,Encoding,Runtime) to pipeline and level.
// When -layers is set, it overrides -pipeline and -level from profile.
func applyLayers(opts *Options) error {
	if opts.Layers == "" {
		return nil
	}
	items := splitCSV(opts.Layers)
	hasAST := false
	hasFlow := false
	hasEncoding := false
	hasRuntime := false
	for _, it := range items {
		switch strings.ToLower(strings.TrimSpace(it)) {
		case "ast":
			hasAST = true
		case "flow":
			hasFlow = true
		case "encoding":
			hasEncoding = true
		case "runtime":
			hasRuntime = true
		default:
			return fmt.Errorf("unknown layer: %s (use: AST,Flow,Encoding,Runtime)", it)
		}
	}
	var pipe []string
	if hasAST {
		pipe = append(pipe, "iden")
		opts.IdenMode = "obf"
	}
	if hasEncoding {
		pipe = append(pipe, "stringdict", "strenc", "numenc", "fmt")
		if opts.StrEnc == "off" {
			opts.StrEnc = "xor"
			if opts.StrKeyHex == "" {
				opts.StrKeyHex = "a1b2c3d4"
			}
		}
		if opts.StringDict == 0 {
			opts.StringDict = 30
		}
		opts.NumEnc = true
		opts.FormatMode = "jitter"
	}
	if hasFlow {
		pipe = append(pipe, "cf", "dead")
		opts.CFOpaque = true
		opts.CFShuffle = true
		if opts.DeadProb == 0 {
			opts.DeadProb = 15
		}
	}
	if hasRuntime {
		pipe = append(pipe, "frag")
		opts.Level = 5
		if opts.FragProfile == "" {
			opts.FragProfile = "profile=pro"
		}
		opts.NoIntegrity = true
	} else if hasEncoding {
		opts.Level = 4
	} else if hasAST {
		opts.Level = 3
	} else {
		opts.Level = 2
	}
	opts.Pipeline = strings.Join(pipe, ",")
	return nil
}

func applyFragProfile(opts *Options) error {
	if opts.FragProfile == "" {
		return nil
	}
	kv := strings.SplitN(opts.FragProfile, "=", 2)
	if len(kv) != 2 || kv[0] != "profile" {
		return fmt.Errorf("invalid -frag value: %s", opts.FragProfile)
	}
	switch strings.ToLower(kv[1]) {
	case "tight":
		if !opts.MinFragSet && !opts.MaxFragSet {
			opts.MinFrag, opts.MaxFrag = 6, 10
		}
	case "medium":
		if !opts.MinFragSet && !opts.MaxFragSet {
			opts.MinFrag, opts.MaxFrag = 10, 18
		}
	case "loose":
		if !opts.MinFragSet && !opts.MaxFragSet {
			opts.MinFrag, opts.MaxFrag = 14, 28
		}
	case "pro":
		// More fragments, variable size: strengthened level 5 obfuscation
		if !opts.MinFragSet && !opts.MaxFragSet {
			opts.MinFrag, opts.MaxFrag = 5, 14
		}
	default:
		return fmt.Errorf("unknown fragment profile: %s (use tight|medium|loose|pro)", kv[1])
	}
	return nil
}

func parseHexKey(h string) ([]byte, error) {
	if h == "" {
		return nil, nil
	}
	if len(h)%2 != 0 {
		return nil, errors.New("hex key length must be even")
	}
	key, err := hex.DecodeString(h)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}
	if len(key) > 0 && len(key) < 4 {
		return nil, fmt.Errorf("key too short (%d bytes, minimum 4 recommended)", len(key))
	}
	// Check for weak patterns (all zeros, all same byte)
	if len(key) > 0 && isWeakKey(key) {
		return key, nil // warn but don't reject — user may have a reason
	}
	return key, nil
}

// isWeakKey returns true if the key has weak patterns (all zeros, repeated byte).
func isWeakKey(key []byte) bool {
	if len(key) == 0 {
		return true
	}
	allSame := true
	for i := 1; i < len(key); i++ {
		if key[i] != key[0] {
			allSame = false
			break
		}
	}
	return allSame
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		pp := strings.TrimSpace(p)
		if pp != "" {
			out = append(out, pp)
		}
	}
	return out
}

// printDocsSummary prints the golden rules, recommended commands, and doc links to stderr.
func printDocsSummary() {
	fmt.Fprintf(os.Stderr, "ObfusPS — Golden Rules (Best Practices)\n\n")
	fmt.Fprintf(os.Stderr, "1. Always validate   — Use -validate for critical scripts\n")
	fmt.Fprintf(os.Stderr, "2. Use context-aware — Enable -context-aware when using IEX, Add-Type, ScriptBlock::Create\n")
	fmt.Fprintf(os.Stderr, "3. Use module-aware  — Enable -module-aware for .psm1 modules with Export-ModuleMember\n")
	fmt.Fprintf(os.Stderr, "4. Pin your seed     — Use -seed N for reproducible builds and regression testing\n")
	fmt.Fprintf(os.Stderr, "5. Profile by use    — safe=compatibility, heavy/redteam=labs; always test\n\n")
	fmt.Fprintf(os.Stderr, "Recommended commands (copy-paste ready):\n\n")
	fmt.Fprintf(os.Stderr, "  Production/CI:\n    obfusps -i script.ps1 -o out.ps1 -profile safe -seed 12345 -validate\n\n")
	fmt.Fprintf(os.Stderr, "  Modules (.psm1):\n    obfusps -i MyModule.psm1 -o out.psm1 -profile balanced -module-aware -use-ast\n\n")
	fmt.Fprintf(os.Stderr, "  Dynamic code (IEX/Add-Type):\n    obfusps -i tool.ps1 -o out.ps1 -profile heavy -context-aware -use-ast\n\n")
	fmt.Fprintf(os.Stderr, "  Red Team/detection:\n    obfusps -i payload.ps1 -o out.ps1 -level 5 -profile redteam -report\n\n")
	fmt.Fprintf(os.Stderr, "  Regression testing:\n    obfusps -i script.ps1 -o out.ps1 -seed 42 -profile safe\n\n")
	fmt.Fprintf(os.Stderr, "Engine vs stub: Go engine never embeds PowerShell; all runtime behavior lives in generated stubs.\n")
	fmt.Fprintf(os.Stderr, "AST: -use-ast enables native PowerShell AST (IEX, Add-Type, Export-ModuleMember); fallback to regex. See docs/ROADMAP.md.\n")
	fmt.Fprintf(os.Stderr, "Docs: docs/BEST_PRACTICES.md | docs/DOCUMENTATION.md | docs/ROADMAP.md\n")
}

func buildPipeline(opts *Options, strKey []byte) ([]Transform, error) {
	var out []Transform
	if opts.AntiReverse {
		out = append(out, &AntiReverseTransform{})
	}
	items := splitCSV(opts.Pipeline)
	for _, it := range items {
		switch strings.ToLower(strings.TrimSpace(it)) {
		case "iden":
			if strings.ToLower(opts.IdenMode) == "obf" || opts.VarRename {
				out = append(out, &IdentifierTransform{})
			}
		case "strenc":
			if opts.StrEnc == "xor" {
				out = append(out, &StringEncryptTransform{Mode: "xor", Key: strKey})
			} else if opts.StrEnc == "rc4" {
				out = append(out, &StringEncryptTransform{Mode: "rc4", Key: strKey})
			}
		case "stringdict":
			if opts.StringDict > 0 {
				out = append(out, &StringDictTransform{Percent: opts.StringDict})
			}
		case "numenc":
			if opts.NumEnc {
				out = append(out, &NumberEncodeTransform{})
			}
		case "fmt":
			if strings.ToLower(opts.FormatMode) == "jitter" {
				out = append(out, &FormatJitterTransform{})
			}
		case "cf":
			if !opts.FlowSafeMode && opts.CFOpaque {
				out = append(out, &CFOpaqueTransform{})
			}
			if !opts.FlowSafeMode && opts.CFShuffle {
				out = append(out, &CFShuffleTransform{})
			}
		case "dead":
			if !opts.FlowSafeMode && opts.DeadProb > 0 {
				out = append(out, &DeadCodeTransform{Prob: opts.DeadProb})
			}
		case "frag":
		default:
			if it != "" {
				return nil, fmt.Errorf("unknown pipeline item: %s", it)
			}
		}
	}
	return out, nil
}
