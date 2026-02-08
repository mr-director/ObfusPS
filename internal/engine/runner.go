package engine

import (
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func Run(opts Options) error {
	if !opts.Quiet {
		fmt.Println(bannerColor)
	}

	// --- Smart Analysis: -recommend / -auto ---
	// Read input early for analysis when auto or recommend is used.
	if opts.Recommend || opts.Auto {
		data, err := readAllInput(opts)
		if err != nil && opts.Recommend {
			return fmt.Errorf("input: %w", err)
		}
		if err == nil {
			if err2 := validateUTF8(data); err2 != nil && opts.Recommend {
				return err2
			}
			if err2 := validateUTF8(data); err2 == nil {
				features := AnalyzeScript(string(data))
				PrintAnalysis(features, opts.Quiet)

				if opts.Recommend {
					return nil // recommend-only mode, no transformation
				}

				// -auto: apply smart recommendations
				if opts.Auto {
					applyAutoSettings(&opts, features)
				}
			}
		}
	}

	if err := applyProfileDefaults(&opts); err != nil {
		return err
	}
	if err := applyLayers(&opts); err != nil {
		return err
	}
	if err := applyFragProfile(&opts); err != nil {
		return err
	}
	if opts.Fuzz > 0 && opts.UseStdout {
		return errors.New("cannot use -fuzz with -stdout")
	}
	if opts.DryRun && opts.UseStdout {
		return errors.New("cannot use -dry-run with -stdout")
	}
	if opts.Validate && opts.UseStdout {
		return errors.New("cannot use -validate with -stdout")
	}
	if opts.Validate && opts.Fuzz > 0 {
		return errors.New("cannot use -validate with -fuzz")
	}
	if err := requireInOut(opts); err != nil {
		return err
	}
	if opts.Level < 1 || opts.Level > 5 {
		return fmt.Errorf("invalid level: %d (valid 1..5)", opts.Level)
	}
	if opts.StringDict < 0 || opts.StringDict > 100 {
		return fmt.Errorf("invalid -stringdict: %d (0..100)", opts.StringDict)
	}
	if opts.DeadProb < 0 || opts.DeadProb > 100 {
		return fmt.Errorf("invalid -deadcode: %d (0..100)", opts.DeadProb)
	}
	if opts.StrEnc != "off" && opts.StrEnc != "xor" && opts.StrEnc != "rc4" {
		return fmt.Errorf("invalid -strenc: %s (off|xor|rc4)", opts.StrEnc)
	}
	if (opts.StrEnc == "xor" || opts.StrEnc == "rc4") && opts.StrKeyHex == "" {
		return errors.New("missing -strkey for -strenc xor|rc4")
	}
	key, err := parseHexKey(opts.StrKeyHex)
	if (opts.StrEnc == "xor" || opts.StrEnc == "rc4") && err != nil {
		return fmt.Errorf("invalid -strkey hex: %w", err)
	}
	if opts.Fuzz > 0 {
		data, err := readAllInput(opts)
		if err != nil {
			return fmt.Errorf("input: %w", err)
		}
		if err := validateUTF8(data); err != nil {
			return err
		}
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "%sFuzz:%s generating %d variants...\n", Cyan, Reset, opts.Fuzz)
		}
		for i := 1; i <= opts.Fuzz; i++ {
			tmp := opts
			tmp.Seeded = true
			tmp.Seed = time.Now().UnixNano() + int64(i*137)
			tmp.Quiet = true // suppress per-variant output
			outName := fuzzOutName(opts.OutputFile, i)
			tmp.OutputFile = outName
			if err := processOnce(tmp, data, key); err != nil {
				return fmt.Errorf("fuzz variant %d/%d failed: %w", i, opts.Fuzz, err)
			}
			if !opts.Quiet {
				fmt.Fprintf(os.Stderr, "%sWrote:%s [%d/%d] %s\n", Green, Reset, i, opts.Fuzz, outName)
			}
		}
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "%sFuzz:%s %d variants generated successfully\n", Green, Reset, opts.Fuzz)
		}
		return nil
	}
	data, err := readAllInput(opts)
	if err != nil {
		return fmt.Errorf("input: %w", err)
	}
	if err := validateUTF8(data); err != nil {
		return err
	}
	if opts.DryRun {
		return runDryRun(opts, data)
	}
	if err := processOnce(opts, data, key); err != nil {
		return err
	}
	if opts.Validate {
		vErr := runValidate(opts)
		if vErr != nil && opts.AutoRetry {
			return runAutoRetry(opts, data, key)
		}
		return vErr
	}
	return nil
}

// applyAutoSettings uses script analysis to automatically configure the best
// obfuscation settings.  Runs BEFORE applyProfileDefaults.
func applyAutoSettings(opts *Options, f *ScriptFeatures) {
	// Only override if user didn't explicitly set these
	if opts.Profile == "" {
		opts.Profile = f.RecommendedProfile
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "%sAuto:%s selected profile %s%s%s\n", Green, Reset, Cyan, opts.Profile, Reset)
		}
	}
	if opts.Level <= 1 {
		opts.Level = f.RecommendedLevel
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "%sAuto:%s selected level %s%d%s\n", Green, Reset, Cyan, opts.Level, Reset)
		}
	}

	// Auto-enable context-aware and AST when needed
	if f.HasDynamicInvoke || f.HasAddType {
		if !opts.ContextAware {
			opts.ContextAware = true
			if !opts.Quiet {
				fmt.Fprintf(os.Stderr, "%sAuto:%s enabled %s-context-aware%s (dynamic code detected)\n", Green, Reset, Cyan, Reset)
			}
		}
		if !opts.UseAST {
			opts.UseAST = true
			if !opts.Quiet {
				fmt.Fprintf(os.Stderr, "%sAuto:%s enabled %s-use-ast%s (AST required for context-aware)\n", Green, Reset, Cyan, Reset)
			}
		}
	}

	// Auto-enable module-aware
	if f.HasModulePatterns && !opts.ModuleAware {
		opts.ModuleAware = true
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "%sAuto:%s enabled %s-module-aware%s (module patterns detected)\n", Green, Reset, Cyan, Reset)
		}
	}

	// Auto-enable validate for complex scripts
	if f.Complexity >= 50 && !opts.Validate {
		opts.Validate = true
		opts.AutoRetry = true
		opts.ValidateStderr = "ignore"
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "%sAuto:%s enabled %s-validate -auto-retry%s (high complexity=%d)\n", Green, Reset, Cyan, Reset, f.Complexity)
		}
	}
}

// runAutoRetry tries progressively safer settings when validation fails.
// Fallback chain: current profile L-1 → balanced → safe.
func runAutoRetry(opts Options, data []byte, key []byte) error {
	type attempt struct {
		profile string
		level   int
		desc    string
	}

	// Build fallback chain
	var chain []attempt

	// Try lowering the level first
	if opts.Level > 3 {
		chain = append(chain, attempt{opts.Profile, opts.Level - 1, fmt.Sprintf("%s L%d", opts.Profile, opts.Level-1)})
	}
	if opts.Level > 1 {
		chain = append(chain, attempt{opts.Profile, 3, fmt.Sprintf("%s L3", opts.Profile)})
	}

	// Try balanced profile
	if opts.Profile != "balanced" && opts.Profile != "safe" {
		chain = append(chain, attempt{"balanced", 4, "balanced L4"})
		chain = append(chain, attempt{"balanced", 3, "balanced L3"})
	}

	// Last resort: safe
	chain = append(chain, attempt{"safe", 3, "safe L3"})

	for i, a := range chain {
		fmt.Fprintf(os.Stderr, "%sAuto-retry:%s [%d/%d] trying %s%s%s...\n",
			Yellow, Reset, i+1, len(chain), Cyan, a.desc, Reset)

		retryOpts := opts
		retryOpts.Profile = a.profile
		retryOpts.Level = a.level
		retryOpts.Pipeline = "" // reset so profile defaults apply
		retryOpts.Quiet = true
		retryOpts.AutoRetry = false // prevent recursion

		if err := applyProfileDefaults(&retryOpts); err != nil {
			continue
		}
		if err := applyFragProfile(&retryOpts); err != nil {
			continue
		}

		retryKey := key
		if (retryOpts.StrEnc == "xor" || retryOpts.StrEnc == "rc4") && retryOpts.StrKeyHex != "" {
			k, err := parseHexKey(retryOpts.StrKeyHex)
			if err == nil {
				retryKey = k
			}
		}

		if err := processOnce(retryOpts, data, retryKey); err != nil {
			continue
		}
		vErr := runValidate(retryOpts)
		if vErr == nil {
			fmt.Fprintf(os.Stderr, "%sAuto-retry:%s %sPASS%s with %s%s%s\n",
				Green, Reset, Green, Reset, Cyan, a.desc, Reset)
			// Print final metrics
			retryOpts.Quiet = false
			return nil
		}
	}

	return fmt.Errorf("auto-retry: all fallback attempts failed — use -profile safe for maximum compatibility")
}

func runDryRun(opts Options, data []byte) error {
	inputLabel := opts.InputFile
	if opts.UseStdin || inputLabel == "" {
		inputLabel = "<stdin>"
	}
	fmt.Fprintf(os.Stderr, "%sDry-run:%s analyzing %s%s%s (%d bytes)\n", Cyan, Reset, Green, inputLabel, Reset, len(data))
	fmt.Fprintf(os.Stderr, "%sProfile:%s %s | %sLevel:%s %d\n", Yellow, Reset, orEmpty(opts.Profile), Yellow, Reset, opts.Level)
	if opts.Layers != "" {
		fmt.Fprintf(os.Stderr, "%sLayers:%s %s\n", Yellow, Reset, opts.Layers)
	}
	if opts.Pipeline != "" {
		fmt.Fprintf(os.Stderr, "%sPipeline:%s %s\n", Yellow, Reset, opts.Pipeline)
	}
	if opts.AntiReverse {
		fmt.Fprintf(os.Stderr, "%sAnti-reverse:%s enabled\n", Yellow, Reset)
	}
	fmt.Fprintf(os.Stderr, "%sNo transformation or output (dry-run).%s\n", Gray, Reset)
	return nil
}

func orEmpty(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func buildAndPrintReport(opts Options, inputData []byte, payload string, m Metrics) {
	inputPath := opts.InputFile
	if opts.UseStdin || inputPath == "" {
		inputPath = "<stdin>"
	}
	outputPath := opts.OutputFile
	if opts.UseStdout {
		outputPath = "<stdout>"
	}
	profile := opts.Profile
	if profile == "" {
		profile = "custom"
	}
	var layers []string
	if opts.Layers != "" {
		for _, p := range splitCSV(opts.Layers) {
			layers = append(layers, strings.TrimSpace(p))
		}
	}
	var techniques []string
	if opts.AntiReverse {
		techniques = append(techniques, "anti-reverse")
	}
	for _, p := range splitCSV(opts.Pipeline) {
		t := strings.TrimSpace(p)
		if t != "" {
			techniques = append(techniques, t)
		}
	}
	techniques = append(techniques, fmt.Sprintf("level%d", opts.Level))
	r := Report{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Profile:    profile,
		Layers:     layers,
		Level:      opts.Level,
		Techniques: techniques,
		InputSize:  len(inputData),
		OutputSize: len(payload),
		Seed:       opts.Seed,
	}
	PrintReport(r, m)
}

func processOnce(opts Options, data []byte, strKey []byte) error {
	// Optional log (write-only, disabled by default)
	var logFile *os.File
	if opts.LogFile != "" {
		f, err := os.Create(opts.LogFile)
		if err != nil {
			return fmt.Errorf("log file: %w", err)
		}
		defer f.Close()
		logFile = f
		inputLabel := opts.InputFile
		if opts.UseStdin || inputLabel == "" {
			inputLabel = "<stdin>"
		}
		fmt.Fprintf(logFile, "[%s] input=%s level=%d\n", time.Now().Format(time.RFC3339), inputLabel, opts.Level)
	}

	// Seed derivation: when -seed not set, seed = hash(script) ^ random (reproducibility unchanged)
	if !opts.Seeded {
		hashSum := SumSha256(data)
		seedBase := int64(binary.LittleEndian.Uint64(hashSum[:8]))
		var b [8]byte
		if _, err := crand.Read(b[:]); err != nil {
			opts.Seed = seedBase ^ time.Now().UnixNano()
		} else {
			opts.Seed = seedBase ^ int64(binary.LittleEndian.Uint64(b[:]))
		}
		opts.Seeded = true
	}

	r := InitRNG(&opts.Seed, opts.Seeded)
	ctx := &Ctx{
		Rng:       r,
		Opts:      &opts,
		InputHash: hex.EncodeToString(SumSha256(data)),
		Helpers:   map[string]bool{},
	}
	ps := string(data)
	// Extract #Requires / Using directives BEFORE transforms — they must stay
	// at the top of the output file.  Transforms like StringDict prepend code
	// ($script:D=@(...)) that would push directives below line 1.
	psDirectives, psBody := extractDirectives(ps)
	ps = psBody

	if opts.UseAST && (opts.ContextAware || opts.ModuleAware) {
		astRes, astErr := RunASTParse(ps)
		if astErr == nil && astRes != nil && astRes.Error == "" {
			ctx.ASTResult = astRes
			if !opts.Quiet {
				fmt.Fprintf(os.Stderr, "%sAST:%s native PowerShell AST loaded successfully\n", Green, Reset)
			}
		} else {
			// AST failed: fall back to regex, warn user
			if !opts.Quiet {
				reason := "unknown"
				if astErr != nil {
					reason = astErr.Error()
				} else if astRes != nil && astRes.Error != "" {
					reason = astRes.Error
				}
				fmt.Fprintf(os.Stderr, "%sWarning:%s AST parsing failed, falling back to regex (%s)\n", Yellow, Reset, reason)
			}
			if logFile != nil {
				fmt.Fprintf(logFile, "[%s] warning: AST fallback to regex: %v\n", time.Now().Format(time.RFC3339), astErr)
			}
		}
	}
	if opts.Pipeline != "" || opts.Profile != "" {
		transforms, err := buildPipeline(&opts, strKey)
		if err != nil {
			if logFile != nil {
				fmt.Fprintf(logFile, "[%s] error: buildPipeline: %v\n", time.Now().Format(time.RFC3339), err)
			}
			return err
		}
		var errT error
		for _, t := range transforms {
			ps, errT = t.Apply(ps, ctx)
			if errT != nil {
				if logFile != nil {
					fmt.Fprintf(logFile, "[%s] error: transform %s: %v\n", time.Now().Format(time.RFC3339), t.Name(), errT)
				}
				return fmt.Errorf("transform %s failed: %w", t.Name(), errT)
			}
		}
	}
	payload, err := obfuscate(ps, opts.Level, opts.NoExec, [2]int{opts.MinFrag, opts.MaxFrag}, opts.NoIntegrity, ctx.Rng)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] error: obfuscate: %v\n", time.Now().Format(time.RFC3339), err)
		}
		return err
	}
	// Re-attach directives that were extracted before transforms.
	if psDirectives != "" {
		payload = psDirectives + "\n" + payload
	}
	payload = payload + "\n# ObfusPS by " + author + " | seed=" + strconv.FormatInt(opts.Seed, 10)
	if logFile != nil {
		outLabel := opts.OutputFile
		if opts.UseStdout {
			outLabel = "<stdout>"
		}
		fmt.Fprintf(logFile, "[%s] seed=%d output=%s\n", time.Now().Format(time.RFC3339), opts.Seed, outLabel)
	}
	m := ComputeMetricsWithInput(payload, len(data))
	if !opts.Quiet {
		fmt.Fprintf(os.Stderr, "%sSeed:%s %d %s(re-run with -seed %d for same output)%s\n", Yellow, Reset, opts.Seed, Gray, opts.Seed, Reset)
		PrintMetrics(m, false)
	}
	if opts.Report {
		buildAndPrintReport(opts, data, payload, m)
	}
	// BOM + leading newline: BOM ensures UTF-8; newline prevents BOM from breaking first token
	// (e.g. [Console]::OutputEncoding) when script runs in Code Runner / some PowerShell contexts.
	outputHeader := []byte{0xEF, 0xBB, 0xBF, '\n'}
	output := append(outputHeader, []byte(payload)...)
	if opts.UseStdout {
		_, err = os.Stdout.Write(output)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputFile), 0755); err != nil && !os.IsNotExist(err) {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] error: mkdir: %v\n", time.Now().Format(time.RFC3339), err)
		}
		return fmt.Errorf("creating output directory: %w", err)
	}
	err = os.WriteFile(opts.OutputFile, output, 0600)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] error: write: %v\n", time.Now().Format(time.RFC3339), err)
		}
		return err
	}
	if !opts.Quiet && !opts.UseStdout {
		fmt.Fprintf(os.Stderr, "%sWrote:%s %s\n", Green, Reset, opts.OutputFile)
	}
	return nil
}

func ObfuscateString(ps string, opts Options) (string, error) {
	r := InitRNG(&opts.Seed, opts.Seeded)
	// Extract directives before transforms (same logic as processOnce)
	psDirectives, psBody := extractDirectives(ps)
	ps = psBody
	ctx := &Ctx{
		Rng:       r,
		Opts:      &opts,
		InputHash: "",
		Helpers:   map[string]bool{},
	}
	if opts.UseAST && (opts.ContextAware || opts.ModuleAware) {
		if astRes, err := RunASTParse(ps); err == nil && astRes.Error == "" {
			ctx.ASTResult = astRes
		}
	}
	key, err := parseHexKey(opts.StrKeyHex)
	if err != nil {
		return "", err
	}
	if (opts.StrEnc == "xor" || opts.StrEnc == "rc4") && (key == nil || len(key) == 0) {
		return "", errors.New("missing -strkey for -strenc xor|rc4")
	}
	if opts.Pipeline != "" || opts.Profile != "" {
		if err := applyProfileDefaults(&opts); err != nil {
			return "", err
		}
		if err := applyFragProfile(&opts); err != nil {
			return "", err
		}
		transforms, err := buildPipeline(&opts, key)
		if err != nil {
			return "", err
		}
		for _, t := range transforms {
			ps, err = t.Apply(ps, ctx)
			if err != nil {
				return "", err
			}
		}
	}
	out, err := obfuscate(ps, opts.Level, opts.NoExec, [2]int{opts.MinFrag, opts.MaxFrag}, opts.NoIntegrity, r)
	if err != nil {
		return "", err
	}
	if psDirectives != "" {
		out = psDirectives + "\n" + out
	}
	return out + "\n# ObfusPS by " + author + " | seed=" + strconv.FormatInt(opts.Seed, 10), nil
}
