package engine

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	mathrand "math/rand"
	"strconv"
	"strings"
)

// randVar generates a random PowerShell variable name (without $).
func randVar(r *mathrand.Rand, n int) string {
	if n < 5 {
		n = 5
	}
	if n > 10 {
		n = 10
	}
	return RandIdent(r, n)
}

// varNameMixed mixes role-based names (semantic family) and random names to reduce "machine-generated" look.
func varNameMixed(r *mathrand.Rand, role string, fallbackLen int) string {
	if r.Intn(2) == 0 {
		return RandVarByRole(r, role)
	}
	return randVar(r, fallbackLen)
}

// execForm runs the payload as a .ps1 script (child) for output identical to the original.
// Always & (invoke) + @args: same scope isolation and same parameters as original.ps1.
func execForm(r *mathrand.Rand, varName string) string {
	v := "$" + varName
	return fmt.Sprintf("& ([scriptblock]::Create(%s)) @args", v)
}

// utf8ConsolePS forces console output to UTF-8 (avoids emoji/unicode mojibake).
const utf8ConsolePS = "[Console]::OutputEncoding = [Text.Encoding]::UTF8; $OutputEncoding = [Text.Encoding]::UTF8; "

// extractDirectives splits leading #Requires and Using statements from the script body.
// PowerShell requires these at the very top of a .ps1 file — they cannot appear inside
// a [scriptblock]::Create() call.  Returns (directives, body).
func extractDirectives(ps string) (string, string) {
	lines := strings.Split(ps, "\n")
	var directives []string
	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(lower, "#requires") {
			// blank line or comment (not #Requires) — keep scanning
			directives = append(directives, line)
			bodyStart = i + 1
			continue
		}
		if strings.HasPrefix(lower, "#requires") ||
			strings.HasPrefix(lower, "using ") {
			directives = append(directives, line)
			bodyStart = i + 1
			continue
		}
		// First real statement — stop scanning
		break
	}
	if len(directives) == 0 {
		return "", ps
	}
	// Check if any actual directives were found (not just blanks/comments)
	hasReal := false
	for _, d := range directives {
		t := strings.ToLower(strings.TrimSpace(d))
		if strings.HasPrefix(t, "#requires") || strings.HasPrefix(t, "using ") {
			hasReal = true
			break
		}
	}
	if !hasReal {
		return "", ps
	}
	dir := strings.Join(directives, "\n")
	body := strings.Join(lines[bodyStart:], "\n")
	return dir, body
}

func obfuscate(ps string, level int, noExec bool, fragRange [2]int, noIntegrity bool, r *mathrand.Rand) (string, error) {
	if r == nil {
		r = mathrand.New(mathrand.NewSource(0))
	}
	// Directives (#Requires, Using) are extracted in processOnce() before transforms
	// and re-attached after obfuscate() returns.  The ps here is the body only.
	body := ps
	runVar := randVar(r, 6)
	switch level {
	case 1:
		payload := charsJoinPayload(body)
		if noExec {
			return payload, nil
		}
		return utf8ConsolePS + fmt.Sprintf("$%s = %s; %s", runVar, payload, execForm(r, runVar)), nil
	case 2:
		enc := base64.StdEncoding.EncodeToString([]byte(body))
		if noExec {
			return enc, nil
		}
		return utf8ConsolePS + fmt.Sprintf("$%s = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); %s", runVar, enc, execForm(r, runVar)), nil
	case 3:
		eVar := randVar(r, 4)
		enc := base64.StdEncoding.EncodeToString([]byte(body))
		if noExec {
			return enc, nil
		}
		return utf8ConsolePS + fmt.Sprintf("$%s = [Convert]::FromBase64String('%s'); $%s = [Text.Encoding]::UTF8.GetString($%s); %s", eVar, enc, runVar, eVar, execForm(r, runVar)), nil
	case 4:
		enc, err := gzipAndB64(body)
		if err != nil {
			return "", err
		}
		if noExec {
			return enc, nil
		}
		cVar := randVar(r, 6)
		bVar := randVar(r, 5)
		msVar := randVar(r, 4)
		gzVar := randVar(r, 4)
		srVar := randVar(r, 4)
		return utf8ConsolePS + fmt.Sprintf("$%s = '%s'; $%s = [Convert]::FromBase64String($%s); $%s = New-Object IO.MemoryStream(,$%s); $%s = New-Object IO.Compression.GzipStream($%s,[IO.Compression.CompressionMode]::Decompress); $%s = New-Object IO.StreamReader($%s,[Text.Encoding]::UTF8); $%s = $%s.ReadToEnd(); %s",
			cVar, enc, bVar, cVar, msVar, bVar, gzVar, msVar, srVar, gzVar, runVar, srVar, execForm(r, runVar)), nil
	case 5:
		// Level 5: GZip + XOR (key derived from seed, no key in clear) + Base64 + fragments + order encoded (B64)
		// Without execution: need seed, derive key, decode order, reassemble, un-XOR, decompress.
		gz, err := gzipBytes(body)
		if err != nil {
			return "", err
		}
		seed := r.Int63() & 0x7FFFFFFF
		key := DeriveKey32FromSeed(seed) // 32 bytes for stronger obfuscation
		payload := make([]byte, len(gz))
		copy(payload, gz)
		xorBytes(payload, key)
		enc := base64.StdEncoding.EncodeToString(payload)
		minFrag, maxFrag := fragRange[0], fragRange[1]
		// Keep fragment count <= 256 (order encoded in one byte per fragment)
		const maxFragments = 256
		minSizeForLimit := (len(enc) + maxFragments - 1) / maxFragments
		if minSizeForLimit > minFrag {
			minFrag = minSizeForLimit
			if maxFrag < minFrag {
				maxFrag = minFrag + 10
			}
		}
		frags := fragment(enc, minFrag, maxFrag, r)
		if len(frags) == 0 {
			return "", fmt.Errorf("fragmentation produced no fragments")
		}
		if len(frags) > maxFragments {
			return "", fmt.Errorf("too many fragments for order encoding (max %d)", maxFragments)
		}
		order := make([]int, len(frags))
		for i := range order {
			order[i] = i
		}
		ShuffleInts(r, order)
		shuffled := make([]string, len(frags))
		for i := range order {
			shuffled[i] = frags[order[i]]
		}
		escaped := escapePSFragments(shuffled)
		joinedArr := "@('" + strings.Join(escaped, "','") + "')"
		orderBytes := make([]byte, len(order))
		for i, o := range order {
			orderBytes[i] = byte(o)
		}
		orderB64 := base64.StdEncoding.EncodeToString(orderBytes)
		orderB64Escaped := strings.ReplaceAll(orderB64, "'", "''")
		if noExec {
			return joinedArr + "; '" + orderB64Escaped + "'; " + strconv.FormatInt(seed, 10), nil
		}
		fVar := randVar(r, 6)
		oBytesVar := randVar(r, 5)
		invVar := randVar(r, 5)
		cVar := randVar(r, 6)
		bVar := randVar(r, 5)
		kVar := randVar(r, 5)
		sVar := varNameMixed(r, "state", 4) // mix role-based / random names
		mVar := randVar(r, 4)
		msVar := randVar(r, 4)
		gzVar := randVar(r, 4)
		srVar := randVar(r, 4)
		hVar := randVar(r, 4)
		maskVar := randVar(r, 4)
		var integrityPS string
		if !noIntegrity {
			// Integrity: if payload is modified (patch), wrong but silent behaviour (logical divergence)
			expectedHash32 := payloadHash32(body)
			storedHash := expectedHash32 ^ integrityMask
			integrityPS = fmt.Sprintf("$%s = %d; $%s = 1515870810; $hAlg = [System.Security.Cryptography.SHA256]::Create(); $hBytes = $hAlg.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($%s)); $actualH = [BitConverter]::ToInt32($hBytes, 0); if($actualH -ne ($%s -bxor $%s)){ $%s = 'exit 0' }",
				hVar, storedHash, maskVar, runVar, hVar, maskVar, runVar)
		}
		// Decoy: fake seed / unused variable (complicates static analysis)
		decoyVar := randVar(r, 5)
		fakeSeed := r.Int63() & 0x7FFFFFFF
		if fakeSeed == seed {
			fakeSeed = (fakeSeed + 1) & 0x7FFFFFFF
		}
		decoyPS := fmt.Sprintf("$%s = %d", decoyVar, fakeSeed)
		// Method name built at runtime (avoids "FromBase64String" signature)
		methodNameExpr := psCharCodes("FromBase64String")
		// 32-byte key derivation LCG (same algo as DeriveKey32FromSeed) to avoid storing key in clear
		keyDerivePS := fmt.Sprintf("$%s = New-Object byte[] 32; $%s = %d -band 0x7FFFFFFF; 0..31 | ForEach-Object { $%s = (([long]$%s * 1103515245 + 12345) -band 0x7FFFFFFF); $%s[$_] = [byte](($%s -shr 16) -band 0xFF) }",
			kVar, sVar, seed, sVar, sVar, kVar, sVar)
		n := len(frags)
		// Order decoded via GetMethod (no "FromBase64String" string in clear)
		decodeOrderPS := fmt.Sprintf("$%s = %s; $%s = [Convert].GetMethod($%s, [Type[]]@([string])).Invoke($null, @('%s'))",
			mVar, methodNameExpr, oBytesVar, mVar, orderB64Escaped)
		// Decode payload with same method ref
		decodePayloadPS := fmt.Sprintf("$%s = [Convert].GetMethod($%s, [Type[]]@([string])).Invoke($null, @($%s))", bVar, mVar, cVar)
		// Syntactic noise (dead branches, unused vars). Keep moderate: too much = slowness + bug risk.
		noise1 := noiseBlock(r, 3)
		noise2 := noiseBlock(r, 3)
		noiseBeforeExec := noiseBlock(r, 2)
		execBody := execForm(r, runVar)
		if noiseBeforeExec != "" {
			execBody = noiseBeforeExec + "; " + execBody
		}
		// Opaque predicate (always true) around execution — same result
		execBody = wrapOpaque(r, execBody)
		// Structural polymorphism: 5 templates for greater variety
		templateKind := r.Intn(5)
		switch templateKind {
		case 1:
			// Nested scriptblock invocation
			execBody = "& { " + execBody + " }"
		case 3:
			// ForEach-Object wrapper (single-iteration loop)
			execBody = fmt.Sprintf("@(1) | ForEach-Object { %s }", execBody)
		case 4:
			// Try-catch wrapper (try always succeeds)
			execBody = fmt.Sprintf("try { %s } catch { }", execBody)
		}
		// templateKind 0 and 2 = linear (default) and intermediate variables (handled below)
		// Order variant: sometimes decode order before key (independent steps, same result)
		var block1, block2 string
		if r.Intn(2) == 0 {
			block1 = keyDerivePS
			block2 = decodeOrderPS
		} else {
			block1 = decodeOrderPS
			block2 = keyDerivePS
		}
		sep1 := "; "
		if noise1 != "" {
			sep1 = "; " + noise1 + "; "
		}
		sep2 := "; "
		if noise2 != "" {
			sep2 = "; " + noise2 + "; "
		}
		// Template "intermediate variables": step join -> midVar then cVar = midVar (same result)
		joinAssign := fmt.Sprintf("$%s = -join (0..%d | ForEach-Object { $%s[$%s[$_]] })", cVar, n-1, fVar, invVar)
		if templateKind == 2 {
			midVar := randVar(r, 5)
			joinAssign = fmt.Sprintf("$%s = -join (0..%d | ForEach-Object { $%s[$%s[$_]] }); $%s = $%s", midVar, n-1, fVar, invVar, cVar, midVar)
		}
		return utf8ConsolePS + fmt.Sprintf("$%s = %s; %s%s%s%s%s; $%s = New-Object int[] %d; 0..%d | ForEach-Object { $%s[$%s[$_]] = $_ }; %s; %s; for($i=0;$i -lt $%s.Length;$i++){ $%s[$i] = $%s[$i] -bxor $%s[$i%%$%s.Length] }; $%s = New-Object IO.MemoryStream(,$%s); $%s = New-Object IO.Compression.GzipStream($%s,[IO.Compression.CompressionMode]::Decompress); $%s = New-Object IO.StreamReader($%s,[Text.Encoding]::UTF8); $%s = $%s.ReadToEnd(); %s; %s",
			fVar, joinedArr, decoyPS, sep1, block1, sep2, block2, invVar, n, n-1, invVar, oBytesVar, joinAssign, decodePayloadPS, bVar, bVar, bVar, kVar, kVar, msVar, bVar, gzVar, msVar, srVar, gzVar, runVar, srVar, integrityPS, execBody), nil
	default:
		return "", fmt.Errorf("unsupported level: %d (valid 1..5)", level)
	}
}

func charsJoinPayload(s string) string {
	nums := make([]string, 0, len(s))
	for _, ch := range s {
		nums = append(nums, strconv.Itoa(int(ch)))
	}
	// Codepoints > 65535 (emojis, etc.) don't fit in [char]; use ConvertFromUtf32 in PS
	return fmt.Sprintf("$(-join ((%s) | ForEach-Object { if ($_ -le 65535) { [char]$_ } else { [char]::ConvertFromUtf32($_) } }))", strings.Join(nums, ","))
}

func gzipAndB64(s string) (string, error) {
	b, err := gzipBytes(s)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// gzipBytes returns the script compressed with gzip.
func gzipBytes(s string) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write([]byte(s)); err != nil {
		_ = gz.Close()
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return b.Bytes(), nil
}

// xorBytes applies repeated XOR with the key (key[i % len(key)]).
func xorBytes(data, key []byte) {
	if len(key) == 0 {
		return
	}
	for i := range data {
		data[i] ^= key[i%len(key)]
	}
}

// Integrity: mask constant for hash (avoids storing hash in clear).
const integrityMask = 0x5A5A5A5A

// payloadHash32 returns the first 4 bytes of SHA256(ps) as int32 (LE). Used for integrity.
func payloadHash32(ps string) int32 {
	h := sha256.Sum256([]byte(ps))
	return int32(binary.LittleEndian.Uint32(h[:4]))
}

func fragment(s string, minFrag, maxFrag int, r *mathrand.Rand) []string {
	if minFrag < 1 {
		minFrag = 1
	}
	if maxFrag < minFrag {
		maxFrag = minFrag + 6
	}
	if len(s) == 0 {
		return nil
	}
	var out []string
	for i := 0; i < len(s); {
		size := maxFrag
		if maxFrag > minFrag && r != nil {
			size = minFrag + r.Intn(maxFrag-minFrag+1)
			if size < minFrag {
				size = minFrag
			}
		} else if maxFrag > minFrag {
			size = minFrag + (len(s)-i)%(maxFrag-minFrag+1)
			if size < minFrag {
				size = minFrag
			}
		}
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		chunk := s[i:end]
		if len(chunk) > 0 {
			out = append(out, chunk)
		}
		i = end
	}
	return out
}

func escapePSFragments(frags []string) []string {
	out := make([]string, len(frags))
	for i, f := range frags {
		out[i] = strings.ReplaceAll(f, "'", "''")
	}
	return out
}

// psCharCodes returns a PS expression that builds string s from character codes (avoids signatures).
func psCharCodes(s string) string {
	if s == "" {
		return "''"
	}
	codes := make([]string, 0, len(s))
	for _, c := range s {
		codes = append(codes, strconv.Itoa(int(c)))
	}
	return "-join ([char[]](" + strings.Join(codes, ",") + "))"
}

// noiseStatement returns a no-effect PS statement (controlled noise: dead computations, branches never taken, overwritten values).
func noiseStatement(r *mathrand.Rand) string {
	switch r.Intn(14) {
	case 0:
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = 0", v)
	case 1:
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = 1 + 1", v)
	case 2:
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = $null", v)
	case 3:
		// Dead computation: value never read
		a, b := randVar(r, 4), randVar(r, 4)
		return fmt.Sprintf("$%s = 3; $%s = $%s * 2", a, b, a)
	case 4:
		// Branch never taken (0 -eq 1)
		v := randVar(r, 4)
		return fmt.Sprintf("if(0 -eq 1){ $%s = 0 }", v)
	case 5:
		// Overwritten value: first assignment unused
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = 1; $%s = 2", v, v)
	case 6:
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = [int]0", v)
	case 7:
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = ''", v)
	case 8:
		// Array creation (never used)
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = @(%d,%d,%d)", v, r.Intn(100), r.Intn(100), r.Intn(100))
	case 9:
		// String concatenation (never used)
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = '%s' + '%s'", v, RandIdent(r, 3), RandIdent(r, 3))
	case 10:
		// Type test (never used)
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = [string]::IsNullOrEmpty('')", v)
	case 11:
		// Hashtable creation (never used)
		v, k := randVar(r, 4), RandIdent(r, 3)
		return fmt.Sprintf("$%s = @{'%s'=%d}", v, k, r.Intn(100))
	case 12:
		// Nested dead branch with computation
		v, w := randVar(r, 4), randVar(r, 4)
		return fmt.Sprintf("if($false){ $%s = [Math]::Sqrt(%d); $%s = $%s }", v, r.Intn(9999), w, v)
	default:
		// Comparison (never used)
		v := randVar(r, 4)
		return fmt.Sprintf("$%s = %d -gt %d", v, r.Intn(100), r.Intn(100)+100)
	}
}

// noiseBlock returns 0 to n noise statements (n depends on RNG).
func noiseBlock(r *mathrand.Rand, max int) string {
	if max < 1 {
		max = 1
	}
	count := r.Intn(max + 1)
	if count == 0 {
		return ""
	}
	var parts []string
	for i := 0; i < count; i++ {
		parts = append(parts, noiseStatement(r))
	}
	return strings.Join(parts, "; ")
}

// opaqueTrue returns a PS condition that is always true (opaque predicate, does not change flow).
func opaqueTrue(r *mathrand.Rand) string {
	switch r.Intn(8) {
	case 0:
		return "1 -eq 1"
	case 1:
		v := randVar(r, 3)
		return fmt.Sprintf("($%s = 1) -eq 1", v)
	case 2:
		return "[int]1 -eq [int]1"
	case 3:
		return "'' -eq ''"
	case 4:
		// Arithmetic: always true
		a := r.Intn(50) + 1
		return fmt.Sprintf("%d -le %d", a, a+r.Intn(50)+1)
	case 5:
		// String length: always true
		return fmt.Sprintf("'%s'.Length -gt 0", RandIdent(r, 3))
	case 6:
		// Type check: always true
		return "$null -eq $null"
	default:
		// Boolean logic: always true
		return "$true -or $false"
	}
}

// wrapOpaque wraps code in if(always true) { code } to obfuscate flow (same result).
func wrapOpaque(r *mathrand.Rand, body string) string {
	cond := opaqueTrue(r)
	return fmt.Sprintf("if(%s){ %s }", cond, body)
}
