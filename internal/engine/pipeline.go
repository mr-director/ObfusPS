package engine

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	mathrand "math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
)


type IdentifierTransform struct{}

func (t *IdentifierTransform) Name() string { return "iden" }

// inSingleQuotedString returns true if position pos is inside a single-quoted string.
// In single-quoted strings, $var is literal text—never rename it.
func inSingleQuotedString(ps string, pos int, sqSpans [][]int) bool {
	for _, span := range sqSpans {
		if pos >= span[0] && pos < span[1] {
			return true
		}
	}
	return false
}

// findExportedFunctions returns function names to protect (Export-ModuleMember -Function).
// When ctx.ASTResult is set (-use-ast), uses native AST; otherwise falls back to regex.
func findExportedFunctions(ps string, ctx *Ctx) map[string]bool {
	if ctx != nil && ctx.ASTResult != nil && len(ctx.ASTResult.ExportedFunctions) > 0 {
		protected := make(map[string]bool)
		for _, fn := range ctx.ASTResult.ExportedFunctions {
			protected[fn] = true
		}
		return protected
	}
	return findExportedFunctionsRegex(ps)
}

func findExportedFunctionsRegex(ps string) map[string]bool {
	protected := map[string]bool{}
	for _, m := range reExportModuleMember.FindAllStringSubmatch(ps, -1) {
		if len(m) < 2 {
			continue
		}
		list := strings.TrimSpace(m[1])
		for _, part := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ';' }) {
			name := strings.TrimSpace(part)
			if name != "" {
				protected[name] = true
			}
		}
	}
	return protected
}

func (t *IdentifierTransform) Apply(ps string, ctx *Ctx) (string, error) {
	mapping := map[string]string{}
	reservedPrefix := "$__"

	// Detect class property names so we never rename them (would break $this.Property).
	classProps := findClassPropertyNames(ps)
	// Detect function parameter names — callers use -ParamName which is never
	// renamed, so the $ParamName variable must keep its original name.
	funcParams := findFunctionParamNames(ps)

	// renameBase renames a simple (non-scoped) variable.
	var renameBase func(string) string
	renameBase = func(name string) string {
		if strings.HasPrefix(name, reservedPrefix) {
			return name
		}
		if isReservedVariable(name) {
			return name
		}
		// Never rename variables that are also class properties.
		if classProps[strings.ToLower(name)] {
			return name
		}
		// Never rename function parameter variables (callers use -Name syntax).
		if funcParams[strings.ToLower(name)] {
			return name
		}
		if v, ok := mapping[name]; ok {
			return v
		}
		n := RandIdent(ctx.Rng, len(name))
		if strings.HasPrefix(name, "$") && !strings.HasPrefix(n, "$") {
			n = "$" + n
		}
		mapping[name] = n
		return n
	}

	// rename handles scoped variables ($script:counter, $global:x, etc.).
	// The scope prefix is preserved; only the base name is renamed.
	rename := func(name string) string {
		lower := strings.ToLower(name)
		if idx := strings.Index(lower, ":"); idx > 0 {
			scopePrefix := name[:idx+1] // e.g. "$script:"
			scope := strings.TrimPrefix(strings.ToLower(scopePrefix), "$")
			scope = strings.TrimSuffix(scope, ":")
			// Environment variables: never rename
			if scope == "env" {
				return name
			}
			// Known scope modifier: rename only the base part
			switch scope {
			case "script", "global", "local", "private", "using", "variable", "workflow":
				baseName := "$" + name[idx+1:] // e.g. "$counter"
				renamed := renameBase(baseName)
				return scopePrefix + renamed[1:] // e.g. "$script:" + "renamedVar"
			}
		}
		return renameBase(name)
	}

	exported := map[string]bool{}
	if ctx.Opts != nil && ctx.Opts.ModuleAware {
		exported = findExportedFunctions(ps, ctx)
	}

	// --- Collect function names from declarations ---
	funcNames := map[string]string{}
	for _, m := range reFuncHeader.FindAllStringSubmatch(ps, -1) {
		fn := m[1]
		if _, ok := funcNames[fn]; !ok {
			if exported[fn] {
				funcNames[fn] = fn
			} else {
				funcNames[fn] = RandIdent(ctx.Rng, len(fn))
			}
		}
	}
	for _, m := range reFuncNoParam.FindAllStringSubmatch(ps, -1) {
		fn := m[1]
		if _, ok := funcNames[fn]; !ok {
			if exported[fn] {
				funcNames[fn] = fn
			} else {
				funcNames[fn] = RandIdent(ctx.Rng, len(fn))
			}
		}
	}

	// --- Position-aware function name replacement ---
	// Build protected regions where function names must NOT be replaced:
	// single-quoted strings, double-quoted strings, here-strings.
	sqIdx := reSQ.FindAllStringIndex(ps, -1)
	dqIdx := reDQ.FindAllStringIndex(ps, -1)
	hsSpans := findHereStringSpans(ps)
	sqIdx = filterOutHereStringsIdx(sqIdx, hsSpans)
	dqIdx = filterOutHereStringsIdx(dqIdx, hsSpans)
	// Remove fake SQ matches that fall inside DQ strings (e.g. "'$var'" inside "...'$var'...")
	{
		var dqEnc [][]int
		dqEnc = append(dqEnc, dqIdx...)
		for _, hs := range hsSpans {
			if hs.s+1 < len(ps) && ps[hs.s+1] == '"' {
				dqEnc = append(dqEnc, []int{hs.s, hs.e})
			}
		}
		sqIdx = filterSQInsideDQ(sqIdx, dqEnc)
	}
	type intSpan struct{ s, e int }
	var funcProtected []intSpan
	// Single-quoted strings: fully protected (no interpolation).
	for _, s := range sqIdx {
		funcProtected = append(funcProtected, intSpan{s[0], s[1]})
	}
	// Double-quoted strings: protect only the literal parts, NOT $(...) subexpressions
	// which contain executable code where function names must be renamed.
	for _, s := range dqIdx {
		for _, sp := range splitDQProtectedSpans(ps, s[0], s[1]) {
			funcProtected = append(funcProtected, intSpan{sp[0], sp[1]})
		}
	}
	// Here-strings: single-quoted @'...'@ fully protected; double-quoted @"..."@ split.
	for _, hs := range hsSpans {
		isSQ := hs.s+1 < len(ps) && ps[hs.s+1] == '\''
		if isSQ {
			funcProtected = append(funcProtected, intSpan{hs.s, hs.e})
		} else {
			for _, sp := range splitDQProtectedSpans(ps, hs.s, hs.e) {
				funcProtected = append(funcProtected, intSpan{sp[0], sp[1]})
			}
		}
	}
	sort.Slice(funcProtected, func(i, j int) bool { return funcProtected[i].s < funcProtected[j].s })

	if len(funcNames) > 0 {
		// Build a combined regex for all function names (longest first for proper matching).
		var funcPatterns []string
		for orig := range funcNames {
			funcPatterns = append(funcPatterns, regexp.QuoteMeta(orig))
		}
		sort.Slice(funcPatterns, func(i, j int) bool {
			return len(funcPatterns[i]) > len(funcPatterns[j])
		})
		combined := regexp.MustCompile(`(?i)\b(?:` + strings.Join(funcPatterns, "|") + `)\b`)
		fnMatches := combined.FindAllStringIndex(ps, -1)

		var buf strings.Builder
		last := 0
		for _, m := range fnMatches {
			buf.WriteString(ps[last:m[0]])
			orig := ps[m[0]:m[1]]

			// Skip if inside a protected string region.
			skip := false
			for _, p := range funcProtected {
				if p.s > m[0] {
					break
				}
				if m[0] >= p.s && m[0] < p.e {
					skip = true
					break
				}
			}
			// Skip method calls (.MethodName) and static calls (::MethodName).
			if !skip && m[0] > 0 && ps[m[0]-1] == '.' {
				skip = true
			}
			if !skip && m[0] > 1 && ps[m[0]-2:m[0]] == "::" {
				skip = true
			}
			// Skip if inside a PowerShell attribute argument.
			if !skip && isInsideAttribute(ps, m[0]) {
				skip = true
			}

			if skip {
				buf.WriteString(orig)
			} else {
				// Case-insensitive lookup for the replacement.
				neo := orig
				for k, v := range funcNames {
					if strings.EqualFold(k, orig) {
						neo = v
						break
					}
				}
				buf.WriteString(neo)
			}
			last = m[1]
		}
		buf.WriteString(ps[last:])
		ps = buf.String()
	}

	// --- Variable replacement ---
	// Recollect spans after function name replacement changed positions.
	// Skip: single-quoted strings, single-quoted here-strings (@'...'@),
	// escaped $, class properties, and attribute arguments.
	// Note: double-quoted here-strings (@"..."@) support variable interpolation,
	// so their variables MUST be renamed (not added to protected spans).
	sqSpans := reSQ.FindAllStringIndex(ps, -1)
	dqEnclosing := buildDQEnclosing(ps)
	sqSpans = filterSQInsideDQ(sqSpans, dqEnclosing) // Remove fake SQ inside DQ strings
	hsSpans2 := findHereStringSpans(ps)
	for _, hs := range hsSpans2 {
		// Only protect single-quoted here-strings (@'...'@)
		if hs.s+1 < len(ps) && ps[hs.s+1] == '\'' {
			sqSpans = append(sqSpans, []int{hs.s, hs.e})
		}
	}
	matches := reVar.FindAllStringIndex(ps, -1)
	var out strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		out.WriteString(ps[last:start])
		v := ps[start:end]
		skip := inSingleQuotedString(ps, start, sqSpans)
		if !skip && start > 0 && ps[start-1] == '`' {
			skip = true // escaped $var in double-quoted string
		}
		// Skip attribute arguments (e.g. [Parameter(Mandatory=$true)])
		if !skip && isInsideAttribute(ps, start) {
			skip = true
		}
		if skip {
			out.WriteString(v)
		} else {
			out.WriteString(rename(v))
		}
		last = end
	}
	out.WriteString(ps[last:])
	ps = out.String()

	// --- Braced variable replacement: ${varname} ---
	// PowerShell allows ${name} syntax for disambiguation (e.g. "${idx}:text").
	// reVar only matches $name, so we need a dedicated pass for ${...} forms.
	reBracedVar := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	bracedMatches := reBracedVar.FindAllStringIndex(ps, -1)
	if len(bracedMatches) > 0 {
		sqSpansBraced := reSQ.FindAllStringIndex(ps, -1)
		dqEnclosingBraced := buildDQEnclosing(ps)
		sqSpansBraced = filterSQInsideDQ(sqSpansBraced, dqEnclosingBraced)
		hsBraced := findHereStringSpans(ps)
		for _, hs := range hsBraced {
			if hs.s+1 < len(ps) && ps[hs.s+1] == '\'' {
				sqSpansBraced = append(sqSpansBraced, []int{hs.s, hs.e})
			}
		}
		var bv strings.Builder
		last = 0
		for _, m := range bracedMatches {
			bv.WriteString(ps[last:m[0]])
			token := ps[m[0]:m[1]]
			innerName := token[2 : len(token)-1]
			dollarForm := "$" + innerName
			// Skip if preceded by backtick (escaped $) or inside single-quoted string
			escaped := m[0] > 0 && ps[m[0]-1] == '`'
			if neo, ok := mapping[dollarForm]; ok && !escaped && !inSingleQuotedString(ps, m[0], sqSpansBraced) {
				bv.WriteString("${" + neo[1:] + "}")
			} else {
				bv.WriteString(token)
			}
			last = m[1]
		}
		bv.WriteString(ps[last:])
		ps = bv.String()
	}

	// --- Splatting replacement ---
	// PowerShell splatting uses @varName instead of $varName.  The variable regex
	// only matches $-prefixed names, so we must apply the same mapping to @-prefixed
	// occurrences.  Build a single-pass replacement over @varName tokens.
	reSplat := regexp.MustCompile(`@[A-Za-z_][A-Za-z0-9_]*`)
	splatMatches := reSplat.FindAllStringIndex(ps, -1)
	if len(splatMatches) > 0 {
		// Recollect literal spans for splatting pass.
		sqSpansSplat := reSQ.FindAllStringIndex(ps, -1)
		dqEnclosingSplat := buildDQEnclosing(ps)
		sqSpansSplat = filterSQInsideDQ(sqSpansSplat, dqEnclosingSplat)
		hsSplat := findHereStringSpans(ps)
		for _, hs := range hsSplat {
			if hs.s+1 < len(ps) && ps[hs.s+1] == '\'' {
				sqSpansSplat = append(sqSpansSplat, []int{hs.s, hs.e})
			}
		}
		var sb strings.Builder
		last = 0
		for _, m := range splatMatches {
			start, end := m[0], m[1]
			sb.WriteString(ps[last:start])
			token := ps[start:end]            // e.g. @splatParams
			dollarForm := "$" + token[1:]     // e.g. $splatParams
			if neo, ok := mapping[dollarForm]; ok && !inSingleQuotedString(ps, start, sqSpansSplat) {
				sb.WriteString("@" + neo[1:]) // e.g. @renamedVar
			} else {
				sb.WriteString(token)
			}
			last = end
		}
		sb.WriteString(ps[last:])
		ps = sb.String()
	}

	return ps, nil
}

// splitDQProtectedSpans takes a double-quoted string region [start, end) and
// returns protected spans that EXCLUDE $(...) subexpressions.  $(...) contains
// executable code where function names must be renamed.
func splitDQProtectedSpans(ps string, start, end int) [][2]int {
	var spans [][2]int
	litStart := start
	i := start
	for i < end-1 {
		// Detect $( subexpression (not escaped by backtick)
		if ps[i] == '$' && i+1 < end && ps[i+1] == '(' {
			if i > 0 && ps[i-1] == '`' {
				i++
				continue
			}
			// Protect the literal part before this $(
			if i > litStart {
				spans = append(spans, [2]int{litStart, i})
			}
			// Find matching ) considering nesting
			depth := 0
			j := i + 1
			for j < end {
				if ps[j] == '(' {
					depth++
				} else if ps[j] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
				j++
			}
			// Skip past the $(...) — this region is NOT protected
			i = j + 1
			litStart = i
			continue
		}
		i++
	}
	// Remaining literal part
	if litStart < end {
		spans = append(spans, [2]int{litStart, end})
	}
	return spans
}

// filterSQInsideDQ removes single-quoted spans that fall entirely within a
// double-quoted string or a double-quoted here-string.  Inside a DQ string,
// single quotes are literal characters and do NOT prevent variable interpolation.
// Without this filter, "$innerB64" inside "...'$innerB64'..." would
// incorrectly protect $innerB64 from renaming.
// dqEnclosing is a combined list of [start,end] pairs covering all DQ regions:
// both single-line DQ strings and DQ here-strings.
func filterSQInsideDQ(sqSpans [][]int, dqEnclosing [][]int) [][]int {
	if len(dqEnclosing) == 0 {
		return sqSpans
	}
	var filtered [][]int
	for _, sq := range sqSpans {
		inside := false
		for _, dq := range dqEnclosing {
			if sq[0] >= dq[0] && sq[1] <= dq[1] {
				inside = true
				break
			}
		}
		if !inside {
			filtered = append(filtered, sq)
		}
	}
	return filtered
}

// buildDQEnclosing builds a combined list of all double-quoted regions:
// single-line DQ strings (from reDQ) and DQ here-strings (from findHereStringSpans).
func buildDQEnclosing(ps string) [][]int {
	dqIdx := reDQ.FindAllStringIndex(ps, -1)
	hsSpans := findHereStringSpans(ps)
	dqIdx = filterOutHereStringsIdx(dqIdx, hsSpans)
	for _, hs := range hsSpans {
		// Double-quoted here-strings: @"..."@
		if hs.s+1 < len(ps) && ps[hs.s+1] == '"' {
			dqIdx = append(dqIdx, []int{hs.s, hs.e})
		}
	}
	return dqIdx
}

// findClassPropertyNames extracts variable names declared as class properties.
// These must NOT be renamed because $this.PropertyName references use the original name
// and dot-access (.PropertyName) is never rewritten by the identifier transform.
func findClassPropertyNames(ps string) map[string]bool {
	props := make(map[string]bool)
	reClass := regexp.MustCompile(`(?i)\bclass\s+\w+[^{]*\{`)
	// Handle Hidden/Static keywords before the type annotation.
	// Use [\w\.\[\], ]+ inside brackets to handle complex nested generics
	// like [HashSet[Tuple[Int32, Int32]]].
	reProp := regexp.MustCompile(`(?mi)^\s*(?:(?:hidden|static)\s+)*(?:\[[\w\.\[\], ]+(?:\(.*?\))?\]\s*)*(\$[A-Za-z_]\w*)`)
	for _, cs := range reClass.FindAllStringIndex(ps, -1) {
		braceStart := cs[1] - 1
		braceEnd := findMatchingBrace(ps, braceStart)
		if braceEnd < 0 {
			continue
		}
		body := ps[cs[1]:braceEnd]
		for _, m := range reProp.FindAllStringSubmatch(body, -1) {
			props[strings.ToLower(m[1])] = true
		}
	}

	// Safety net: collect ALL properties accessed via $this.PropertyName.
	// Dot-access member names (.PropertyName) are never renamed, so the
	// corresponding $PropertyName variable (the declaration) must also be
	// preserved.  This catches Hidden/Static properties and complex types
	// that the declaration regex may miss.
	reThisProp := regexp.MustCompile(`\$this\.([A-Za-z_]\w*)`)
	for _, m := range reThisProp.FindAllStringSubmatch(ps, -1) {
		props["$"+strings.ToLower(m[1])] = true
	}

	return props
}

// findFunctionParamNames collects parameter variable names from function declarations
// and param() blocks.  Callers invoke functions with -ParamName <value>, but the
// transform never rewrites the "-ParamName" token, so $ParamName must keep its name.
func findFunctionParamNames(ps string) map[string]bool {
	params := make(map[string]bool)
	// Match variables inside function signature parentheses: function Foo([Type]$p1, $p2)
	reFuncSig := regexp.MustCompile(`(?i)\bfunction\s+[\w-]+\s*\(([^)]*)\)`)
	// Match param(...) blocks (used in advanced functions / script-level)
	reParamBlock := regexp.MustCompile(`(?is)\bparam\s*\(`)
	reVar := regexp.MustCompile(`\$([A-Za-z_]\w*)`)

	// Function signature params
	for _, m := range reFuncSig.FindAllStringSubmatch(ps, -1) {
		for _, v := range reVar.FindAllStringSubmatch(m[1], -1) {
			params["$"+strings.ToLower(v[1])] = true
		}
	}
	// param() block params — find balanced parens
	for _, idx := range reParamBlock.FindAllStringIndex(ps, -1) {
		// Find the '(' right after 'param'
		parenStart := -1
		for i := idx[0]; i < idx[1]; i++ {
			if ps[i] == '(' {
				parenStart = i
				break
			}
		}
		if parenStart < 0 {
			continue
		}
		depth := 0
		parenEnd := -1
		for i := parenStart; i < len(ps); i++ {
			if ps[i] == '(' {
				depth++
			}
			if ps[i] == ')' {
				depth--
				if depth == 0 {
					parenEnd = i
					break
				}
			}
		}
		if parenEnd < 0 {
			continue
		}
		body := ps[parenStart+1 : parenEnd]
		for _, v := range reVar.FindAllStringSubmatch(body, -1) {
			params["$"+strings.ToLower(v[1])] = true
		}
	}
	return params
}

type StringDictTransform struct {
	Percent int
}

func (t *StringDictTransform) Name() string { return "stringdict" }

func (t *StringDictTransform) Apply(ps string, ctx *Ctx) (string, error) {
	dq := reDQ.FindAllStringIndex(ps, -1)
	sq := reSQ.FindAllStringIndex(ps, -1)

	// Filter out regex matches that overlap with here-string regions.
	// reSQ/reDQ produce WRONG partial matches inside here-strings because
	// here-string content can contain unescaped quotes.
	hsSpans := findHereStringSpans(ps)
	dq = filterOutHereStringsIdx(dq, hsSpans)
	sq = filterOutHereStringsIdx(sq, hsSpans)

	type span struct {
		s, e int
		dbl  bool
	}
	var spans []span
	for _, p := range dq {
		spans = append(spans, span{p[0], p[1], true})
	}
	for _, p := range sq {
		spans = append(spans, span{p[0], p[1], false})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].s < spans[j].s })

	// Remove overlapping spans to prevent cursor > sp.s panic
	{
		if len(spans) > 1 {
			deduped := []span{spans[0]}
			for i := 1; i < len(spans); i++ {
				last := deduped[len(deduped)-1]
				if spans[i].s >= last.e {
					deduped = append(deduped, spans[i])
				}
			}
			spans = deduped
		}
	}

	var excluded [][]int
	if ctx.Opts != nil && ctx.Opts.ContextAware {
		excluded = findExecutableStringSpans(ps, ctx)
	}

	var tokens []string
	tokenMap := map[string]int{}
	buildTokens := func(s string) []int {
		var idxs []int
		minTok := 3
		for i := 0; i < len(s); {
			left := len(s) - i
			if left <= minTok {
				chunk := s[i:]
				idxs = append(idxs, addTok(chunk, &tokens, tokenMap))
				break
			}
			maxTok := 6
			size := ctx.Rng.Intn(maxTok-minTok+1) + minTok
			if size > left {
				size = left
			}
			chunk := s[i : i+size]
			idxs = append(idxs, addTok(chunk, &tokens, tokenMap))
			i += size
		}
		return idxs
	}
	var out strings.Builder
	cursor := 0
	var injected bool
	for _, sp := range spans {
		if sp.s < cursor || sp.e > len(ps) || sp.s >= sp.e {
			continue // skip overlapping or invalid spans
		}
		out.WriteString(ps[cursor:sp.s])
		raw := ps[sp.s:sp.e]
		// Skip here-strings and DQ strings with variable interpolation.
		// Also skip strings inside $() subexpressions of DQ strings
		// (the DQ regex can't handle nested quotes in subexpressions).
		if shouldSkipStringTransform(ps, sp.s, sp.e) || isInsideDQSubexpression(ps, sp.s) {
			out.WriteString(raw)
			cursor = sp.e
			continue
		}
		lit := raw
		if len(lit) >= 2 && strings.HasPrefix(lit, "\"") && strings.HasSuffix(lit, "\"") {
			lit = lit[1 : len(lit)-1]
		} else if len(lit) >= 2 && strings.HasPrefix(lit, "'") && strings.HasSuffix(lit, "'") {
			lit = lit[1 : len(lit)-1]
		}
		skipCtx := ctx.Opts != nil && ctx.Opts.ContextAware && spanOverlapsExcluded(sp.s, sp.e, excluded)
		if t.Percent > 0 && len(lit) >= 10 && !skipCtx && ctx.Rng.Intn(100) < t.Percent {
			idxs := buildTokens(lit)
			var parts []string
			for _, id := range idxs {
				parts = append(parts, fmt.Sprintf("$script:D[%d]", id))
			}
			// Use -join instead of + to avoid numeric addition when tokens are digit strings.
			out.WriteString("(-join @(" + strings.Join(parts, ",") + "))")
			injected = true
		} else {
			out.WriteString(raw)
		}
		cursor = sp.e
	}
	out.WriteString(ps[cursor:])
	res := out.String()
	if injected && len(tokens) > 0 {
		var sb strings.Builder
		sb.WriteString("$script:D=@(")
		for i, tk := range tokens {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("'" + strings.ReplaceAll(tk, "'", "''") + "'")
		}
		sb.WriteString(");\n")
		res = sb.String() + res
	}
	return res, nil
}

func addTok(t string, arr *[]string, idx map[string]int) int {
	if id, ok := idx[t]; ok {
		return id
	}
	id := len(*arr)
	*arr = append(*arr, t)
	idx[t] = id
	return id
}

type StringEncryptTransform struct {
	Mode string
	Key  []byte
}

func (t *StringEncryptTransform) Name() string { return "strenc" }

func (t *StringEncryptTransform) Apply(ps string, ctx *Ctx) (string, error) {
	if len(t.Key) == 0 {
		return ps, fmt.Errorf("strenc: encryption key is empty (use -strkey)")
	}
	if t.Mode == "xor" {
		return encryptStrings(ps, ctx,
			func(b []byte) (enc string, helper string) {
				xb := make([]byte, len(b))
				for i := range b {
					xb[i] = b[i] ^ t.Key[i%len(t.Key)]
				}
				enc = base64.StdEncoding.EncodeToString(xb)
				return enc, ""
			},
			func(enc string) string {
				khex := strings.ToUpper(hex.EncodeToString(t.Key))
				return fmt.Sprintf(
					`(&{[byte[]]$k=0..(%d-1)|%%{[Convert]::ToByte('%s'.Substring($_*2,2),16)};`+
						`[byte[]]$b=[Convert]::FromBase64String('%s');`+
						`for($i=0;$i -lt $b.Length;$i++){$b[$i]=$b[$i] -bxor $k[$i%%$k.Length]};`+
						`[Text.Encoding]::UTF8.GetString($b)})`,
					len(t.Key), khex, enc)
			})
	}
	if t.Mode == "rc4" {
		fn := "__dec" + RandIdent(ctx.Rng, 6)
		if !ctx.Helpers["rc4"] {
			rc4Func := fmt.Sprintf(
				`function %s($k,[byte[]]$d){$s=0..255;$j=0;for($i=0;$i -lt 256;$i++){`+
					`$j=($j+$s[$i]+$k[$i%%$k.Length])%%256;$t=$s[$i];$s[$i]=$s[$j];$s[$j]=$t}`+
					`$i=0;$j=0;for($x=0;$x -lt $d.Length;$x++){`+
					`$i=($i+1)%%256;$j=($j+$s[$i])%%256;$t=$s[$i];$s[$i]=$s[$j];$s[$j]=$t;`+
					`$d[$x]=$d[$x] -bxor $s[($s[$i]+$s[$j])%%256]}`+
					`[Text.Encoding]::UTF8.GetString($d)}`, fn)
			ps = rc4Func + "\n" + ps
			ctx.Helpers["rc4"] = true
		}
		khex := strings.ToUpper(hex.EncodeToString(t.Key))
		return encryptStrings(ps, ctx,
			func(b []byte) (string, string) {
				s := make([]byte, 256)
				for i := 0; i < 256; i++ {
					s[i] = byte(i)
				}
				j := 0
				for i := 0; i < 256; i++ {
					j = (j + int(s[i]) + int(t.Key[i%len(t.Key)])) % 256
					s[i], s[j] = s[j], s[i]
				}
				i, j2 := 0, 0
				enc := make([]byte, len(b))
				for x := 0; x < len(b); x++ {
					i = (i + 1) % 256
					j2 = (j2 + int(s[i])) % 256
					s[i], s[j2] = s[j2], s[i]
					keystream := s[(int(s[i])+int(s[j2]))%256]
					enc[x] = b[x] ^ keystream
				}
				return base64.StdEncoding.EncodeToString(enc), ""
			},
			func(enc string) string {
				return fmt.Sprintf(
					`(%s ([byte[]](0..(%d-1)|%%{[Convert]::ToByte('%s'.Substring($_*2,2),16)})) ([Convert]::FromBase64String('%s')))`,
					fn, len(t.Key), khex, enc)
			})
	}
	return ps, nil
}

// reExecutableContext matches positions before string args of IEX, Add-Type, ScriptBlock::Create.
var reExecutableContext = regexp.MustCompile(`(?i)(?:Invoke-Expression|\[ScriptBlock\]::Create|Add-Type\s+-TypeDefinition|Add-Type\s+-MemberDefinition)\s*[\(\s]+|\biex\s+[\(\s]*`)

// findExecutableStringSpans returns [][]int{start,end} for string literals that are code (IEX, Add-Type, etc.).
// When ctx.ASTResult is set (-use-ast), uses native AST; otherwise falls back to regex.
func findExecutableStringSpans(ps string, ctx *Ctx) [][]int {
	if ctx != nil && ctx.ASTResult != nil && len(ctx.ASTResult.ExecutableStrings) > 0 {
		return ctx.ASTResult.ExecutableStrings
	}
	return findExecutableStringSpansRegex(ps)
}

func findExecutableStringSpansRegex(ps string) [][]int {
	matches := reExecutableContext.FindAllStringIndex(ps, -1)
	if len(matches) == 0 {
		return nil
	}
	var excluded [][]int
	const maxLook = 400
	for _, m := range matches {
		after := m[1]
		if after >= len(ps) {
			continue
		}
		// Find next string literal (dq or sq) within maxLook chars
		searchEnd := after + maxLook
		if searchEnd > len(ps) {
			searchEnd = len(ps)
		}
		region := ps[after:searchEnd]
		dq := reDQ.FindStringIndex(region)
		sq := reSQ.FindStringIndex(region)
		var first []int
		if dq != nil && (sq == nil || dq[0] <= sq[0]) {
			first = []int{after + dq[0], after + dq[1]}
		} else if sq != nil {
			first = []int{after + sq[0], after + sq[1]}
		}
		if first != nil {
			excluded = append(excluded, first)
		}
	}
	return excluded
}

func spanOverlapsExcluded(s, e int, excluded [][]int) bool {
	for _, ex := range excluded {
		if s < ex[1] && e > ex[0] {
			return true
		}
	}
	return false
}

func encryptStrings(ps string, ctx *Ctx, encfn func([]byte) (string, string), psExpr func(string) string) (string, error) {
	idxs := reDQ.FindAllStringIndex(ps, -1)
	idxs2 := reSQ.FindAllStringIndex(ps, -1)

	// Filter out regex matches that overlap with here-string regions.
	hsSpans := findHereStringSpans(ps)
	idxs = filterOutHereStringsIdx(idxs, hsSpans)
	idxs2 = filterOutHereStringsIdx(idxs2, hsSpans)

	var spans []spanSE
	for _, p := range idxs {
		spans = append(spans, spanSE{p[0], p[1]})
	}
	for _, p := range idxs2 {
		spans = append(spans, spanSE{p[0], p[1]})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].s < spans[j].s })

	// Remove overlapping spans: when DQ and SQ regexes match overlapping regions,
	// keep only non-overlapping spans (skip any span that starts before the previous one ends).
	spans = deduplicateSpans(spans)

	var excluded [][]int
	if ctx.Opts != nil && ctx.Opts.ContextAware {
		excluded = findExecutableStringSpans(ps, ctx)
	}

	// Protect the $D=@(...) token array produced by StringDictTransform.
	// When stringdict runs before strenc (correct order), the script contains
	// $D=@('tok1','tok2',...); — encrypting these short tokens would break the array.
	dStart, dEnd := findDArrayRegion(ps)

	var out strings.Builder
	cursor := 0
	for _, sp := range spans {
		if sp.s < cursor {
			continue // safety: skip if overlapping (should not happen after dedup)
		}
		if sp.e > len(ps) || sp.s >= sp.e {
			continue // safety: skip invalid span
		}
		out.WriteString(ps[cursor:sp.s])
		raw := ps[sp.s:sp.e]
		if len(raw) < 2 {
			out.WriteString(raw)
			cursor = sp.e
			continue
		}
		// Skip strings inside the $D=@(...) preamble (token dictionary from stringdict)
		if dStart >= 0 && sp.s >= dStart && sp.e <= dEnd {
			out.WriteString(raw)
			cursor = sp.e
			continue
		}
		// Skip strings that must not be transformed:
		// - here-strings (@'...'@, @"..."@): transforming loses @ delimiters
		// - DQ strings with variable interpolation ("Hello $name"): transforming loses expansion
		// - executable strings (IEX, Add-Type): transforming breaks dynamic code
		if shouldSkipStringTransform(ps, sp.s, sp.e) || spanOverlapsExcluded(sp.s, sp.e, excluded) || isInsideDQSubexpression(ps, sp.s) {
			out.WriteString(raw)
		} else {
			lit := raw[1 : len(raw)-1]
			enc, _ := encfn([]byte(lit))
			out.WriteString(psExpr(enc))
		}
		cursor = sp.e
	}
	out.WriteString(ps[cursor:])
	return out.String(), nil
}

type spanSE struct{ s, e int }

// findDArrayRegion locates the $D=@(...);\n preamble produced by StringDictTransform.
// Returns (start, end) byte offsets of the entire declaration, or (-1,-1) if absent.
// strenc must skip all strings inside this region to avoid corrupting the token array.
func findDArrayRegion(ps string) (int, int) {
	prefix := "$script:D=@("
	start := strings.Index(ps, prefix)
	if start < 0 {
		return -1, -1
	}
	// Walk from opening ( tracking depth; honour single-quoted strings.
	depth := 1
	inSQ := false
	for i := start + len(prefix); i < len(ps); i++ {
		ch := ps[i]
		if ch == '\'' {
			if inSQ {
				// Escaped '' (doubled single-quote) inside SQ string
				if i+1 < len(ps) && ps[i+1] == '\'' {
					i++
					continue
				}
				inSQ = false
			} else {
				inSQ = true
			}
			continue
		}
		if inSQ {
			continue
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				end := i + 1
				if end < len(ps) && ps[end] == ';' {
					end++
				}
				if end < len(ps) && ps[end] == '\n' {
					end++
				}
				return start, end
			}
		}
	}
	return -1, -1
}

// deduplicateSpans filters a sorted span list to keep only non-overlapping spans.
// When two spans overlap, the first one (earlier start) wins.
func deduplicateSpans(spans []spanSE) []spanSE {
	if len(spans) == 0 {
		return spans
	}
	result := []spanSE{spans[0]}
	for i := 1; i < len(spans); i++ {
		last := result[len(result)-1]
		if spans[i].s >= last.e {
			result = append(result, spans[i])
		}
		// else: overlapping, skip
	}
	return result
}

// isHereString returns true if the string span at [start,end) in ps is part of
// a PowerShell here-string (@"..."@ or @'...'@). Here-strings must not be
// transformed because the @ delimiters are outside the span and would be left
// dangling, producing invalid PowerShell syntax.
func isHereString(ps string, start, end int) bool {
	if start > 0 && ps[start-1] == '@' {
		return true
	}
	if end < len(ps) && ps[end] == '@' {
		return true
	}
	return false
}

// findHereStringSpans locates all PowerShell here-string regions in ps.
// A here-string starts with @' or @" at the end of a line (followed by newline)
// and ends with '@ or "@ at the start of a line.
// Returns spans covering the full region including the @ delimiters.
func findHereStringSpans(ps string) []spanSE {
	var spans []spanSE
	i := 0
	for i < len(ps)-2 {
		if ps[i] == '@' && (ps[i+1] == '\'' || ps[i+1] == '"') {
			quoteChar := ps[i+1]
			// Verify @' / @" is followed by newline (optional \r before \n)
			afterQ := i + 2
			if afterQ < len(ps) && ps[afterQ] == '\r' {
				afterQ++
			}
			if afterQ >= len(ps) || ps[afterQ] != '\n' {
				i++
				continue
			}
			// Determine terminator: '@ or "@
			var term string
			if quoteChar == '\'' {
				term = "'@"
			} else {
				term = `"@`
			}
			// Search for terminator at the start of a line
			searchFrom := afterQ + 1
			found := false
			for j := searchFrom; j <= len(ps)-len(term); j++ {
				atLineStart := (j == 0) || ps[j-1] == '\n'
				if atLineStart && ps[j] == term[0] && ps[j+1] == term[1] {
					endPos := j + len(term)
					spans = append(spans, spanSE{i, endPos})
					i = endPos
					found = true
					break
				}
			}
			if !found {
				i++
			}
		} else {
			i++
		}
	}
	return spans
}

// filterOutHereStrings removes string spans that overlap with here-string regions.
func filterOutHereStrings(spans []spanSE, hsSpans []spanSE) []spanSE {
	if len(hsSpans) == 0 {
		return spans
	}
	var result []spanSE
	for _, sp := range spans {
		overlaps := false
		for _, hs := range hsSpans {
			if sp.s < hs.e && sp.e > hs.s {
				overlaps = true
				break
			}
		}
		if !overlaps {
			result = append(result, sp)
		}
	}
	return result
}

// filterOutHereStringsIdx removes [][]int spans that overlap with here-string regions.
func filterOutHereStringsIdx(spans [][]int, hsSpans []spanSE) [][]int {
	if len(hsSpans) == 0 {
		return spans
	}
	var result [][]int
	for _, sp := range spans {
		overlaps := false
		for _, hs := range hsSpans {
			if sp[0] < hs.e && sp[1] > hs.s {
				overlaps = true
				break
			}
		}
		if !overlaps {
			result = append(result, sp)
		}
	}
	return result
}

// containsInterpolation returns true if a double-quoted string literal
// (the content between the quotes, not including the quotes themselves)
// contains unescaped PowerShell variable references ($var, $(expr), etc.).
// Transforming such strings (encryption, tokenization) would lose the
// interpolation and change the script's behavior.
func containsInterpolation(lit string) bool {
	for i := 0; i < len(lit); i++ {
		if lit[i] != '$' {
			continue
		}
		// Skip escaped $ (backtick escape: `$)
		if i > 0 && lit[i-1] == '`' {
			continue
		}
		// Check if $ is followed by a variable-like start character
		if i+1 < len(lit) {
			ch := lit[i+1]
			if ch == '(' || ch == '{' || ch == '_' ||
				(ch >= 'A' && ch <= 'Z') ||
				(ch >= 'a' && ch <= 'z') {
				return true
			}
		}
	}
	return false
}

// isInsideAttribute returns true if position `start` is inside a PowerShell
// attribute argument list like [Alias('x')], [ValidateSet('a','b')], etc.
// Attribute arguments must be constants — expressions like $script:D[n] are invalid.
func isInsideAttribute(ps string, start int) bool {
	// Walk backwards on the same line to find (
	for i := start - 1; i >= 0; i-- {
		ch := ps[i]
		if ch == '\n' || ch == '\r' {
			return false
		}
		if ch == '(' {
			// Check if preceded by [Word pattern (attribute invocation)
			for j := i - 1; j >= 0; j-- {
				c := ps[j]
				if c == '[' {
					return true // [AttributeName(
				}
				if c == '\n' || c == '\r' || c == ';' || c == '=' {
					return false
				}
				if c == ' ' || c == '\t' {
					continue
				}
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
					continue
				}
				return false
			}
		}
	}
	return false
}

// shouldSkipStringTransform returns true if a string span should not be
// encrypted or tokenized because doing so would break the script.
// Reasons: here-string syntax, double-quoted string with variable interpolation,
// attribute argument context.
// isInsideDQSubexpression checks if position pos is inside a $() subexpression
// that itself is inside a double-quoted string.  The DQ regex cannot handle
// nested quotes inside $() — e.g. "outer $(code 'inner') more" — so strings
// found inside such subexpressions should NOT be tokenized or encrypted.
func isInsideDQSubexpression(ps string, pos int) bool {
	// Scan backwards from pos to find an unmatched $( that sits inside a DQ string.
	depth := 0
	for i := pos - 1; i >= 1; i-- {
		if ps[i] == ')' {
			depth++
		} else if ps[i] == '(' && ps[i-1] == '$' {
			if depth > 0 {
				depth--
			} else {
				// Found unmatched $( — check if there's an opening " before it
				// that isn't closed before the $(
				for j := i - 2; j >= 0; j-- {
					if ps[j] == '"' && (j == 0 || ps[j-1] != '`') {
						return true // inside a DQ string's $() subexpression
					}
					if ps[j] == '\n' || ps[j] == '\r' {
						break // hit newline without finding opening quote
					}
				}
				return false
			}
		}
	}
	return false
}

func shouldSkipStringTransform(ps string, start, end int) bool {
	if start < 0 || end > len(ps) || start >= end {
		return true
	}
	if isHereString(ps, start, end) {
		return true
	}
	raw := ps[start:end]
	if len(raw) < 2 {
		return true
	}
	// Double-quoted strings with interpolation or backtick escapes must be preserved.
	// Encrypting "`n" would produce the literal chars backtick+n instead of a newline.
	if raw[0] == '"' {
		lit := raw[1 : len(raw)-1]
		if containsInterpolation(lit) {
			return true
		}
		// Backtick escapes (`n, `t, `r, `0, `$, ``, etc.) are evaluated at parse time.
		// Encrypting the raw escape sequence and decrypting at runtime returns the
		// literal sequence, not the intended character.  Skip these.
		if strings.ContainsRune(lit, '`') {
			return true
		}
	}
	// Strings inside attribute argument lists must remain constants
	if isInsideAttribute(ps, start) {
		return true
	}
	return false
}

type NumberEncodeTransform struct{}

func (t *NumberEncodeTransform) Name() string { return "numenc" }

func shouldSkipNumberContext(s string, start, end int) bool {
	// Redirection operators: 2>&1, etc.
	if end < len(s) && end+1 <= len(s) && strings.HasPrefix(s[end:], ">&") {
		return true
	}
	if start >= 2 {
		pfx := s[start-2 : start]
		if pfx == ">&" || pfx == "<&" {
			return true
		}
	}
	// Decimal numbers: 3.14, .5, 0.1
	if (start > 0 && s[start-1] == '.') || (end < len(s) && s[end] == '.') {
		return true
	}
	// Scientific notation: 4.56e-7, 1e+3, 2E10
	if start >= 2 {
		ch := s[start-1]
		if ch == '-' || ch == '+' {
			if start >= 3 && (s[start-2] == 'e' || s[start-2] == 'E') {
				return true
			}
		}
		if ch == 'e' || ch == 'E' {
			return true
		}
	}
	// Hex literals: 0x1A, 0xFF
	if start >= 2 && (s[start-2:start] == "0x" || s[start-2:start] == "0X") {
		return true
	}
	if start >= 1 && end < len(s) && (s[end] == 'x' || s[end] == 'X') && s[start:end] == "0" {
		return true
	}
	// Version-like patterns: 5.1, 7.0 (after a dot or before a dot — already handled above)
	// Array indices: $arr[3] — OK to encode, PowerShell supports expressions in []
	return false
}

func encodeNumber(n int, r *mathrand.Rand) string {
	c := r.Intn(5) + 1
	b := r.Intn(0x7FFF)
	a := (n + c) ^ b
	return fmt.Sprintf("((0x%X -bxor 0x%X)-%d)", a, b, c)
}

func replaceNumsSafe(seg string, r *mathrand.Rand) string {
	var out strings.Builder
	last := 0
	idxs := reNum.FindAllStringIndex(seg, -1)
	for _, p := range idxs {
		start, end := p[0], p[1]
		if shouldSkipNumberContext(seg, start, end) {
			out.WriteString(seg[last:end])
			last = end
			continue
		}
		n, err := strconv.Atoi(seg[start:end])
		if err != nil {
			out.WriteString(seg[last:end])
			last = end
			continue
		}
		encoded := encodeNumber(n, r)

		// Detect negative-argument context: if preceded by '-' and that '-'
		// is preceded by whitespace/SOL, this is a negative number passed as
		// a function argument (e.g. "& $fn -3").  PowerShell cannot parse
		// "-((expr))" so we emit "(-N_encoded)" wrapping the whole thing.
		if start > 0 && seg[start-1] == '-' {
			beforeMinus := start - 1
			isNegArg := beforeMinus == 0
			if !isNegArg && beforeMinus > 0 {
				ch := seg[beforeMinus-1]
				isNegArg = ch == ' ' || ch == '\t' || ch == '(' || ch == ',' || ch == '='
			}
			if isNegArg {
				// Rewind to include the '-' sign
				out.WriteString(seg[last : start-1])
				out.WriteString("(-" + encoded + ")")
				last = end
				continue
			}
		}

		out.WriteString(seg[last:start])
		out.WriteString(encoded)
		last = end
	}
	out.WriteString(seg[last:])
	return out.String()
}

func (t *NumberEncodeTransform) Apply(ps string, ctx *Ctx) (string, error) {
	dqIdx := reDQ.FindAllStringIndex(ps, -1)
	sqIdx := reSQ.FindAllStringIndex(ps, -1)

	// Filter out regex matches that overlap with here-string regions,
	// and add here-string spans as protected regions so numbers inside
	// here-string content are not transformed.
	hsSpans := findHereStringSpans(ps)
	dqIdx = filterOutHereStringsIdx(dqIdx, hsSpans)
	sqIdx = filterOutHereStringsIdx(sqIdx, hsSpans)

	var spans []spanSE
	for _, p := range dqIdx {
		spans = append(spans, spanSE{p[0], p[1]})
	}
	for _, p := range sqIdx {
		spans = append(spans, spanSE{p[0], p[1]})
	}
	// Add here-string regions as "string" spans so their content is protected
	for _, hs := range hsSpans {
		spans = append(spans, hs)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].s < spans[j].s })

	// Remove overlapping spans to prevent cursor > sp.s panic
	spans = deduplicateSpans(spans)

	var out strings.Builder
	cursor := 0
	for _, sp := range spans {
		if sp.s < cursor || sp.e > len(ps) || sp.s >= sp.e {
			continue // skip overlapping or invalid spans
		}
		before := ps[cursor:sp.s]
		before = replaceNumsSafe(before, ctx.Rng)
		out.WriteString(before)
		out.WriteString(ps[sp.s:sp.e])
		cursor = sp.e
	}
	rest := ps[cursor:]
	rest = replaceNumsSafe(rest, ctx.Rng)
	out.WriteString(rest)
	return out.String(), nil
}

type FormatJitterTransform struct{}

func (t *FormatJitterTransform) Name() string { return "fmt" }

func (t *FormatJitterTransform) Apply(ps string, ctx *Ctx) (string, error) {
	lines := strings.Split(ps, "\n")

	// Mark lines inside here-strings (@'...'@ and @"..."@).
	// Here-string content and terminators MUST NOT be modified:
	// - '@ / "@ must start at column 0 (no added space)
	// - content is literal (no trimming or padding)
	// - no extra newlines between here-string lines
	inHS := make([]bool, len(lines))
	insideHS := false
	var hsTerm string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if insideHS {
			inHS[i] = true
			if trimmed == hsTerm {
				insideHS = false
			}
		} else {
			if strings.HasSuffix(trimmed, "@'") {
				inHS[i] = true
				hsTerm = "'@"
				insideHS = true
			} else if strings.HasSuffix(trimmed, `@"`) {
				inHS[i] = true
				hsTerm = `"@`
				insideHS = true
			}
		}
	}

	for i := range lines {
		if inHS[i] {
			continue // never touch here-string content or delimiters
		}
		// Never modify lines ending with backtick ` (line continuation).
		// Trimming or adding whitespace after ` would break the continuation.
		trimmed := strings.TrimRight(lines[i], " \t")
		if strings.HasSuffix(trimmed, "`") {
			continue
		}
		if ctx.Rng.Intn(100) < 35 {
			lines[i] = strings.TrimSpace(lines[i])
		}
		if ctx.Rng.Intn(100) < 30 {
			lines[i] = " " + lines[i]
		}
		if ctx.Rng.Intn(100) < 30 {
			lines[i] = lines[i] + " "
		}
	}

	// Join: preserve exact single newlines between here-string lines and after
	// backtick continuations to avoid changing their content; use random newlines elsewhere.
	var out strings.Builder
	for i, line := range lines {
		if i > 0 {
			prevTrimmed := strings.TrimRight(lines[i-1], " \t")
			hasContinuation := strings.HasSuffix(prevTrimmed, "`")
			if inHS[i] || inHS[i-1] || hasContinuation {
				out.WriteString("\n")
			} else {
				out.WriteString(strings.Repeat("\n", 1+ctx.Rng.Intn(2)))
			}
		}
		out.WriteString(line)
	}
	return out.String(), nil
}

type CFOpaqueTransform struct{}

func (t *CFOpaqueTransform) Name() string { return "cf-opaque" }

func (t *CFOpaqueTransform) Apply(ps string, ctx *Ctx) (string, error) {
	return fmt.Sprintf("if(1 -eq 1){\n%s\n}", ps), nil
}

type CFShuffleTransform struct{}

func (t *CFShuffleTransform) Name() string { return "cf-shuffle" }

var reBlankOrComment = regexp.MustCompile(`(?m)^[\t ]*(?:#.*)?$`)

func (t *CFShuffleTransform) Apply(ps string, ctx *Ctx) (string, error) {
	type fb struct{ start, end int }
	var blocks []fb
	locs := reFuncNoParam.FindAllStringIndex(ps, -1)
	for _, st := range locs {
		start := st[0]
		end := findMatchingBrace(ps, start)
		if end > start {
			blocks = append(blocks, fb{start, end})
		}
	}
	if len(blocks) < 2 {
		return ps, nil // nothing to shuffle
	}

	// Identify contiguous groups of functions.  Two consecutive functions belong
	// to the same group if the text between them contains ONLY blank lines and
	// comments (no executable statements).  Only groups of 2+ functions are
	// shuffled, which avoids breaking scripts that have executable code between
	// function definitions (since that code may depend on the functions above it).
	type group struct {
		indices []int // indices into blocks[]
	}
	var groups []group
	cur := group{indices: []int{0}}
	for i := 1; i < len(blocks); i++ {
		between := ps[blocks[i-1].end:blocks[i].start]
		lines := strings.Split(between, "\n")
		onlyBlank := true
		for _, ln := range lines {
			if !reBlankOrComment.MatchString(ln) {
				onlyBlank = false
				break
			}
		}
		if onlyBlank {
			cur.indices = append(cur.indices, i)
		} else {
			groups = append(groups, cur)
			cur = group{indices: []int{i}}
		}
	}
	groups = append(groups, cur)

	// Build output: copy the script as-is but shuffle within each group.
	var buf strings.Builder
	cursor := 0
	for _, g := range groups {
		if len(g.indices) < 2 {
			// Single function, no shuffling — emit as-is
			last := g.indices[0]
			buf.WriteString(ps[cursor:blocks[last].end])
			cursor = blocks[last].end
			continue
		}
		// Emit everything before the first function in this group
		first := g.indices[0]
		buf.WriteString(ps[cursor:blocks[first].start])

		// Collect functions in this group
		var funcs []string
		for _, idx := range g.indices {
			funcs = append(funcs, ps[blocks[idx].start:blocks[idx].end])
		}
		RandPerm(ctx.Rng, funcs)

		// Re-emit shuffled functions with the original inter-function whitespace
		for i, fn := range funcs {
			buf.WriteString(fn)
			if i < len(funcs)-1 {
				// Use the glue (whitespace/comments) between the original i-th and (i+1)-th
				gIdxA := g.indices[i]
				gIdxB := g.indices[i+1]
				buf.WriteString(ps[blocks[gIdxA].end:blocks[gIdxB].start])
			}
		}
		cursor = blocks[g.indices[len(g.indices)-1]].end
	}
	buf.WriteString(ps[cursor:])
	return buf.String(), nil
}

func findMatchingBrace(s string, start int) int {
	i := strings.Index(s[start:], "{")
	if i < 0 {
		return -1
	}
	depth := 0
	inSQ := false // inside single-quoted string
	inDQ := false // inside double-quoted string
	for pos := start + i; pos < len(s); pos++ {
		ch := s[pos]
		// Track single-quoted strings (no escape sequences)
		if ch == '\'' && !inDQ {
			if inSQ {
				// Check for escaped '' (doubled single-quote)
				if pos+1 < len(s) && s[pos+1] == '\'' {
					pos++ // skip ''
					continue
				}
				inSQ = false
			} else {
				inSQ = true
			}
			continue
		}
		// Track double-quoted strings (backtick escapes)
		if ch == '"' && !inSQ {
			if inDQ {
				inDQ = false
			} else {
				inDQ = true
			}
			continue
		}
		// Skip backtick-escaped characters inside DQ strings
		if ch == '`' && inDQ && pos+1 < len(s) {
			pos++ // skip escaped char
			continue
		}
		// Only count braces outside of strings
		if !inSQ && !inDQ {
			switch ch {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return pos + 1
				}
			}
		}
	}
	return -1
}

type DeadCodeTransform struct{ Prob int }

func (t *DeadCodeTransform) Name() string { return "deadcode" }

func (t *DeadCodeTransform) Apply(ps string, ctx *Ctx) (string, error) {
	if ctx.Rng.Intn(100) >= t.Prob {
		return ps, nil
	}
	fn := "__obfDC" + RandIdent(ctx.Rng, 8)
	// Use long, prefixed variable names to virtually eliminate collision with user variables.
	loopVar := "_dc" + RandIdent(ctx.Rng, 8)
	deadVar1 := "_dc" + RandIdent(ctx.Rng, 8)
	deadVar2 := "_dc" + RandIdent(ctx.Rng, 8)
	snippets := []string{
		fmt.Sprintf("function %s{ return }", fn),
		fmt.Sprintf("for($%s=0;$%s -lt 0;$%s++){Start-Sleep -Milliseconds 0}", loopVar, loopVar, loopVar),
		fmt.Sprintf("$%s='canary';$%s=$%s+$%s|Out-Null", deadVar1, deadVar2, deadVar1, deadVar1),
	}
	var out strings.Builder
	out.WriteString(ps)
	for _, s := range snippets {
		if ctx.Rng.Intn(100) < t.Prob {
			out.WriteString("\n" + s + "\n")
		}
	}
	return out.String(), nil
}

func regexpQuoteWord(word string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
}

// AntiReverseTransform prepends anti-debug/sandbox checks to the script.
// Runs first so the check executes before the main payload.
type AntiReverseTransform struct{}

func (t *AntiReverseTransform) Name() string { return "anti-reverse" }

func (t *AntiReverseTransform) Apply(ps string, ctx *Ctx) (string, error) {
	v := RandIdent(ctx.Rng, 4)
	// Anti-debug: exit if debugger attached (common in analysis environments)
	// Uses variable to avoid trivial signature matching
	check := fmt.Sprintf("$%s=[System.Diagnostics.Debugger]::IsAttached;if($%s){exit 0};", v, v)
	return check + ps, nil
}
