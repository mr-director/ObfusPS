package engine

import (
	"math/rand"
	"strings"
	"testing"
)

const simpleScript = "Write-Output 'Hello'; $x = 1 + 1; Write-Output $x"

// TestObfuscateLevels checks that all levels 1-5 produce output without error.
func TestObfuscateLevels(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for level := 1; level <= 5; level++ {
		out, err := obfuscate(simpleScript, level, false, [2]int{10, 20}, false, r)
		if err != nil {
			t.Errorf("level %d: %v", level, err)
			continue
		}
		if len(out) == 0 {
			t.Errorf("level %d: empty output", level)
		}
	}
}

// TestDeterminism checks that same seed produces same output (reproducibility).
func TestDeterminism(t *testing.T) {
	opts := Options{Level: 5, NoExec: false, MinFrag: 10, MaxFrag: 20, Seeded: true, Seed: 12345}
	out1, err := ObfuscateString(simpleScript, opts)
	if err != nil {
		t.Fatal(err)
	}
	opts2 := opts
	out2, err := ObfuscateString(simpleScript, opts2)
	if err != nil {
		t.Fatal(err)
	}
	if out1 != out2 {
		t.Error("same seed should produce identical output")
	}
}

// TestSeedInOutput checks that output contains the seed (reproducibility, logging).
func TestSeedInOutput(t *testing.T) {
	opts := Options{Level: 3, NoExec: false, Seeded: false, Seed: 0}
	out, err := ObfuscateString(simpleScript, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "seed=") {
		t.Error("output should contain seed= in footer for reproducibility")
	}
	if !strings.Contains(out, "ObfusPS by") {
		t.Error("output should contain ObfusPS signature")
	}
}

// TestSingleQuotedLiteral ensures $var inside single-quoted strings is NOT renamed (literal text).
func TestSingleQuotedLiteral(t *testing.T) {
	script := `$x=1; Write-Output 'User: $username'`
	r := rand.New(rand.NewSource(99))
	ctx := &Ctx{Rng: r, Opts: &Options{IdenMode: "obf"}, Helpers: map[string]bool{}}
	iden := &IdentifierTransform{}
	out, err := iden.Apply(script, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "$username") {
		t.Error("$username inside single-quoted string must not be renamed (literal text)")
	}
}

// TestReservedVariables ensures automatic variables are never renamed.
func TestReservedVariables(t *testing.T) {
	script := `param($x); Write-Host $MyInvocation; $args | ForEach-Object { $_ }`
	r := rand.New(rand.NewSource(99))
	ctx := &Ctx{Rng: r, Opts: &Options{IdenMode: "obf"}, Helpers: map[string]bool{}}
	iden := &IdentifierTransform{}
	out, err := iden.Apply(script, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "MyInvocation") {
		t.Error("$MyInvocation must not be renamed")
	}
	if !strings.Contains(out, "$args") {
		t.Error("$args must not be renamed")
	}
}

// TestFuzzOutName ensures case-insensitive .ps1 handling and correct output names.
func TestFuzzOutName(t *testing.T) {
	tests := []struct {
		base string
		i    int
		want string
	}{
		{"script.ps1", 1, "script.v1.ps1"},
		{"script.PS1", 2, "script.v2.ps1"},
		{"Script.Ps1", 3, "Script.v3.ps1"},
		{"out/sub.ps1", 1, "out/sub.v1.ps1"},
		{"", 1, "obfuscated.v1.ps1"},
		{"noext", 1, "noext.v1.ps1"},
	}
	for _, tt := range tests {
		got := fuzzOutName(tt.base, tt.i)
		if got != tt.want {
			t.Errorf("fuzzOutName(%q, %d) = %q, want %q", tt.base, tt.i, got, tt.want)
		}
	}
}

// TestSafeProfile ensures safe profile produces output.
func TestSafeProfile(t *testing.T) {
	opts := Options{Profile: "safe"}
	out, err := ObfuscateString(simpleScript, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Error("safe profile must produce non-empty output")
	}
}

// TestObfuscateStringEmptyKey ensures StrEnc xor/rc4 without key returns error, not panic.
func TestObfuscateStringEmptyKey(t *testing.T) {
	opts := Options{Level: 2, StrEnc: "xor", StrKeyHex: "", Pipeline: "iden,strenc"}
	_, err := ObfuscateString(simpleScript, opts)
	if err == nil || !strings.Contains(err.Error(), "strkey") {
		t.Errorf("expected error about missing strkey, got: %v", err)
	}
}

// TestModuleAware ensures Export-ModuleMember functions are not renamed.
func TestModuleAware(t *testing.T) {
	script := `function Get-Foo { return 1 }; function Get-Bar { return 2 }; Export-ModuleMember -Function Get-Foo, Get-Bar`
	r := rand.New(rand.NewSource(99))
	opts := &Options{IdenMode: "obf", ModuleAware: true}
	ctx := &Ctx{Rng: r, Opts: opts, Helpers: map[string]bool{}}
	iden := &IdentifierTransform{}
	out, err := iden.Apply(script, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Get-Foo") || !strings.Contains(out, "Get-Bar") {
		t.Error("module-aware: exported functions Get-Foo, Get-Bar must not be renamed")
	}
}

// TestContextAware skips strenc for strings in Invoke-Expression.
func TestContextAware(t *testing.T) {
	script := `Invoke-Expression "Write-Output 1"`
	r := rand.New(rand.NewSource(42))
	opts := &Options{StrEnc: "xor", ContextAware: true}
	ctx := &Ctx{Rng: r, Opts: opts, Helpers: map[string]bool{}}
	strenc := &StringEncryptTransform{Mode: "xor", Key: []byte{0xa1, 0xb2, 0xc3, 0xd4}}
	out, err := strenc.Apply(script, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The IEX string "Write-Output 1" should remain unencrypted
	if !strings.Contains(out, "Write-Output 1") {
		t.Error("context-aware: string in Invoke-Expression should not be encrypted")
	}
}

// TestFragProfile ensures -frag profile sets MinFrag/MaxFrag when not explicitly passed.
func TestFragProfile(t *testing.T) {
	opts := Options{Level: 5, Profile: "light", FragProfile: "profile=tight", MinFragSet: false, MaxFragSet: false}
	if err := applyProfileDefaults(&opts); err != nil {
		t.Fatal(err)
	}
	if err := applyFragProfile(&opts); err != nil {
		t.Fatal(err)
	}
	if opts.MinFrag != 6 || opts.MaxFrag != 10 {
		t.Errorf("profile=tight: got MinFrag=%d MaxFrag=%d, want 6, 10", opts.MinFrag, opts.MaxFrag)
	}
}

// --- Additional tests for better coverage ---

// TestInvalidLevel ensures invalid levels return an error.
func TestInvalidLevel(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for _, level := range []int{0, -1, 6, 100} {
		_, err := obfuscate(simpleScript, level, false, [2]int{10, 20}, false, r)
		if err == nil {
			t.Errorf("level %d: expected error for invalid level", level)
		}
	}
}

// TestEmptyInput ensures empty input is handled gracefully.
func TestEmptyInput(t *testing.T) {
	err := validateUTF8([]byte{})
	if err == nil {
		t.Error("empty input should return an error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

// TestInvalidUTF8 ensures invalid UTF-8 is rejected.
func TestInvalidUTF8(t *testing.T) {
	invalid := []byte{0xFF, 0xFE, 0x00, 0x01}
	err := validateUTF8(invalid)
	if err == nil {
		t.Error("invalid UTF-8 should return an error")
	}
}

// TestXorBytesEmpty ensures xorBytes handles empty key gracefully (no panic).
func TestXorBytesEmpty(t *testing.T) {
	data := []byte("hello")
	xorBytes(data, nil)
	if string(data) != "hello" {
		t.Error("xorBytes with nil key should be a no-op")
	}
	xorBytes(data, []byte{})
	if string(data) != "hello" {
		t.Error("xorBytes with empty key should be a no-op")
	}
}

// TestXorBytesRoundTrip ensures XOR encryption is reversible.
func TestXorBytesRoundTrip(t *testing.T) {
	original := []byte("Write-Output 'Hello World'")
	data := make([]byte, len(original))
	copy(data, original)
	key := []byte{0xAB, 0xCD, 0xEF, 0x12}
	xorBytes(data, key)
	if string(data) == string(original) {
		t.Error("XOR should change the data")
	}
	xorBytes(data, key)
	if string(data) != string(original) {
		t.Error("double XOR should restore original data")
	}
}

// TestDeriveKey32Deterministic ensures key derivation is deterministic.
func TestDeriveKey32Deterministic(t *testing.T) {
	k1 := DeriveKey32FromSeed(12345)
	k2 := DeriveKey32FromSeed(12345)
	if len(k1) != 32 || len(k2) != 32 {
		t.Error("key should be 32 bytes")
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Errorf("key byte %d differs: %d vs %d", i, k1[i], k2[i])
		}
	}
	// Different seeds should produce different keys
	k3 := DeriveKey32FromSeed(99999)
	same := true
	for i := range k1 {
		if k1[i] != k3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different seeds should produce different keys")
	}
}

// TestFragmentation ensures fragment function produces correct results.
func TestFragmentation(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	input := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	frags := fragment(input, 3, 6, r)
	if len(frags) == 0 {
		t.Fatal("fragmentation should produce at least one fragment")
	}
	// Reassemble and verify
	var reassembled strings.Builder
	for _, f := range frags {
		reassembled.WriteString(f)
	}
	if reassembled.String() != input {
		t.Errorf("reassembled fragments don't match: got %q, want %q", reassembled.String(), input)
	}
	// Verify each fragment is within size bounds (last fragment may be smaller)
	for i, f := range frags {
		if i < len(frags)-1 && (len(f) < 3 || len(f) > 7) {
			t.Errorf("fragment %d size %d out of bounds [3,7]", i, len(f))
		}
	}
}

// TestFragmentEmptyInput ensures fragment handles empty input.
func TestFragmentEmptyInput(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	frags := fragment("", 5, 10, r)
	if frags != nil {
		t.Error("empty input should produce nil fragments")
	}
}

// TestGzipRoundTrip ensures gzip compression works correctly.
func TestGzipRoundTrip(t *testing.T) {
	original := "Write-Output 'Hello World'; $x = 42"
	compressed, err := gzipBytes(original)
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) == 0 {
		t.Error("compressed output should not be empty")
	}
}

// TestNumberEncode ensures encodeNumber produces valid PowerShell expressions.
func TestNumberEncode(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for _, n := range []int{0, 1, 42, 100, 255, 65535} {
		result := encodeNumber(n, r)
		if !strings.Contains(result, "-bxor") {
			t.Errorf("encodeNumber(%d) = %q, expected -bxor expression", n, result)
		}
		if !strings.HasPrefix(result, "((") || !strings.HasSuffix(result, ")") {
			t.Errorf("encodeNumber(%d) = %q, expected wrapped expression", n, result)
		}
	}
}

// TestBuildValidateArgs ensures argument parsing handles various cases.
func TestBuildValidateArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"-Name foo", []string{"-Name", "foo"}},
		{`-Name "foo bar"`, []string{"-Name", "foo bar"}},
		{`-Name 'foo bar'`, []string{"-Name", "foo bar"}},
		{`-A "hello" -B 'world'`, []string{"-A", "hello", "-B", "world"}},
		{`"unclosed quote`, []string{"unclosed quote"}},
	}
	for _, tt := range tests {
		got := buildValidateArgs(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("buildValidateArgs(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("buildValidateArgs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// TestDeadCodeTransformNoBreak ensures dead code transform doesn't break the script structure.
func TestDeadCodeTransformNoBreak(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	ctx := &Ctx{Rng: r, Opts: &Options{}, Helpers: map[string]bool{}}
	dead := &DeadCodeTransform{Prob: 100}
	out, err := dead.Apply(simpleScript, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Original script should still be present
	if !strings.Contains(out, "Write-Output") {
		t.Error("dead code transform should preserve original script")
	}
	// Output should be longer (dead code added)
	if len(out) <= len(simpleScript) {
		t.Error("dead code transform with 100% prob should add code")
	}
}

// TestCFOpaqueTransform ensures opaque predicate wrapping.
func TestCFOpaqueTransform(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	ctx := &Ctx{Rng: r, Opts: &Options{}, Helpers: map[string]bool{}}
	cf := &CFOpaqueTransform{}
	out, err := cf.Apply(simpleScript, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "if(1 -eq 1)") {
		t.Error("CF opaque should wrap in if(1 -eq 1)")
	}
	if !strings.Contains(out, simpleScript) {
		t.Error("CF opaque should preserve the original script inside")
	}
}

// TestStringDictTransform ensures string tokenization works.
func TestStringDictTransform(t *testing.T) {
	script := `Write-Output "Hello World from ObfusPS"`
	r := rand.New(rand.NewSource(42))
	ctx := &Ctx{Rng: r, Opts: &Options{}, Helpers: map[string]bool{}}
	sd := &StringDictTransform{Percent: 100}
	out, err := sd.Apply(script, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// With 100% probability and a string >= 10 chars, should be tokenized
	if !strings.Contains(out, "$script:D=@(") {
		t.Error("string dict should create $script:D array for strings >= 10 chars")
	}
	if !strings.Contains(out, "$script:D[") {
		t.Error("string dict should reference tokens via $script:D[i]")
	}
}

// TestFormatJitterTransform ensures format jitter modifies output.
func TestFormatJitterTransform(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	ctx := &Ctx{Rng: r, Opts: &Options{}, Helpers: map[string]bool{}}
	fjt := &FormatJitterTransform{}
	multiLine := "line1\nline2\nline3\nline4\nline5"
	out, err := fjt.Apply(multiLine, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Output should differ from input (random spaces/newlines added)
	if out == multiLine {
		t.Error("format jitter should modify the output")
	}
}

// TestReservedVariableComprehensive ensures all critical reserved variables are protected.
func TestReservedVariableComprehensive(t *testing.T) {
	reserved := []string{"$null", "$true", "$false", "$args", "$PSScriptRoot", "$PSCommandPath", "$_", "$input", "$this", "$Error"}
	for _, v := range reserved {
		if !isReservedVariable(v) {
			t.Errorf("%s should be reserved", v)
		}
	}
	// Non-reserved should return false
	nonReserved := []string{"$myVar", "$customName", "$foo"}
	for _, v := range nonReserved {
		if isReservedVariable(v) {
			t.Errorf("%s should NOT be reserved", v)
		}
	}
}

// TestSafeProfilePreservesStructure verifies safe profile doesn't add transforms.
func TestSafeProfilePreservesStructure(t *testing.T) {
	opts := Options{Profile: "safe", Seeded: true, Seed: 42}
	out, err := ObfuscateString(simpleScript, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Safe profile should NOT contain pipeline transforms artifacts
	if strings.Contains(out, "$D=@(") {
		t.Error("safe profile should not add string dict")
	}
	// Should contain the seed footer
	if !strings.Contains(out, "seed=") {
		t.Error("safe profile output should contain seed footer")
	}
}

// TestAllProfilesValid ensures all profiles can be applied without error.
func TestAllProfilesValid(t *testing.T) {
	profiles := []string{"safe", "light", "balanced", "heavy", "stealth", "paranoid", "redteam", "blueteam", "size", "dev"}
	for _, p := range profiles {
		opts := Options{Profile: p, Level: 5, Seeded: true, Seed: 42}
		_, err := ObfuscateString(simpleScript, opts)
		if err != nil {
			t.Errorf("profile %q failed: %v", p, err)
		}
	}
}

// TestPayloadHash32Deterministic ensures payload hash is deterministic.
func TestPayloadHash32Deterministic(t *testing.T) {
	h1 := payloadHash32("test payload")
	h2 := payloadHash32("test payload")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	h3 := payloadHash32("different payload")
	if h1 == h3 {
		t.Error("different input should produce different hash (unlikely collision)")
	}
}

// TestEscapePSFragments ensures single quotes are properly escaped.
func TestEscapePSFragments(t *testing.T) {
	input := []string{"hello", "it's", "test''double"}
	out := escapePSFragments(input)
	if out[0] != "hello" {
		t.Errorf("expected 'hello', got %q", out[0])
	}
	if out[1] != "it''s" {
		t.Errorf("expected 'it''s', got %q", out[1])
	}
	if out[2] != "test''''double" {
		t.Errorf("expected 'test''''double', got %q", out[2])
	}
}

// TestContainsInterpolation verifies detection of variable references in DQ strings.
func TestContainsInterpolation(t *testing.T) {
	tests := []struct {
		lit  string
		want bool
	}{
		{"Hello World", false},                // no variable
		{"Hello $name", true},                 // variable
		{"Value: $(1+1)", true},               // subexpression
		{"Path: ${env:PATH}", true},           // braced variable
		{"Price: `$5", false},                 // escaped $ (literal)
		{"Count: $count items", true},         // variable
		{"No dollar", false},                  // no $
		{"Trailing $", false},                 // $ at end, no following char
		{"Hello $_", true},                    // $_ (underscore variable)
	}
	for _, tt := range tests {
		got := containsInterpolation(tt.lit)
		if got != tt.want {
			t.Errorf("containsInterpolation(%q) = %v, want %v", tt.lit, got, tt.want)
		}
	}
}

// TestIsHereString ensures here-string detection works.
func TestIsHereString(t *testing.T) {
	// @"..."@ pattern: the regex captures from " to ", so @ is at start-1 or end
	script := `$x = @"` + "\n" + `Hello World` + "\n" + `"@`
	// Find the " positions
	dqIdx := strings.Index(script, "\"")
	lastDq := strings.LastIndex(script, "\"")
	if dqIdx < 0 || lastDq < 0 {
		t.Fatal("could not find quotes in test script")
	}
	if !isHereString(script, dqIdx, lastDq+1) {
		t.Error("should detect here-string (@ before opening quote)")
	}
	// Normal string (no @)
	normal := `Write-Output "hello"`
	nIdx := strings.Index(normal, "\"")
	nEnd := strings.LastIndex(normal, "\"")
	if isHereString(normal, nIdx, nEnd+1) {
		t.Error("should NOT detect here-string for normal DQ string")
	}
}

// TestStrEncSkipsInterpolation ensures string encryption preserves DQ strings with $var.
func TestStrEncSkipsInterpolation(t *testing.T) {
	script := `$name = "John"; Write-Output "Hello $name"`
	r := rand.New(rand.NewSource(42))
	ctx := &Ctx{Rng: r, Opts: &Options{StrEnc: "xor"}, Helpers: map[string]bool{}}
	strenc := &StringEncryptTransform{Mode: "xor", Key: []byte{0xa1, 0xb2, 0xc3, 0xd4}}
	out, err := strenc.Apply(script, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// "Hello $name" has interpolation -> must NOT be encrypted
	if !strings.Contains(out, `"Hello $name"`) {
		t.Error("strenc should preserve DQ strings with variable interpolation")
	}
	// "John" has no interpolation -> CAN be encrypted (may or may not be, depends on impl)
}

// TestStringDictSkipsInterpolation ensures string tokenization preserves DQ strings with $var.
func TestStringDictSkipsInterpolation(t *testing.T) {
	script := `Write-Output "The result is: $result value"`
	r := rand.New(rand.NewSource(42))
	ctx := &Ctx{Rng: r, Opts: &Options{}, Helpers: map[string]bool{}}
	sd := &StringDictTransform{Percent: 100}
	out, err := sd.Apply(script, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// String has interpolation ($result) -> must NOT be tokenized
	if !strings.Contains(out, `"The result is: $result value"`) {
		t.Error("stringdict should preserve DQ strings with variable interpolation")
	}
	// No $D array should be created since the only eligible string was skipped
	if strings.Contains(out, "$D=@(") {
		t.Error("stringdict should not create $D array when all strings are skipped")
	}
}

// TestDeadCodeUsesRandomVars ensures dead code snippets use random variable names.
func TestDeadCodeUsesRandomVars(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	ctx := &Ctx{Rng: r, Opts: &Options{}, Helpers: map[string]bool{}}
	dead := &DeadCodeTransform{Prob: 100}
	out, err := dead.Apply("Write-Output 'test'", ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Should NOT contain hardcoded $i, $x, $y variable names
	// (dead code for-loop should use random var names)
	if strings.Contains(out, "for($i=") {
		t.Error("dead code should NOT use hardcoded $i variable")
	}
	if strings.Contains(out, "$x='canary'") {
		t.Error("dead code should NOT use hardcoded $x variable")
	}
}

// TestAnalyzeScript ensures script analysis detects features correctly.
func TestAnalyzeScript(t *testing.T) {
	script := `
class MyClass { [string]$Name }
enum Color { Red; Green; Blue }
function Test-Thing {
    [CmdletBinding()] param([string]$x)
    $splat = @{Name='test'}
    Test-Thing @splat
    Invoke-Expression 'dir'
    Add-Type -TypeDefinition 'using System;'
    $hash = [System.Security.Cryptography.SHA256]::Create()
    Get-WmiObject Win32_OS
    try { $x } catch { $_ }
    $items | ForEach-Object { $_ }
    $json = ConvertTo-Json @{}
    [xml]$doc = '<root/>'
    Start-Job { 1 }
    Get-Content test.txt
    "${idx}:value"
    "Hello {0}" -f 'World'
}
@'
here string
'@
`
	f := AnalyzeScript(script)
	if !f.HasClasses {
		t.Error("should detect classes")
	}
	if !f.HasEnums {
		t.Error("should detect enums")
	}
	if !f.HasHereStrings {
		t.Error("should detect here-strings")
	}
	if !f.HasDynamicInvoke {
		t.Error("should detect dynamic invoke")
	}
	if !f.HasAddType {
		t.Error("should detect Add-Type")
	}
	if !f.HasCrypto {
		t.Error("should detect crypto")
	}
	if !f.HasWMI {
		t.Error("should detect WMI")
	}
	if !f.HasCmdletBinding {
		t.Error("should detect CmdletBinding")
	}
	if !f.HasErrorHandling {
		t.Error("should detect error handling")
	}
	if !f.HasClosures {
		t.Error("should detect closures")
	}
	if !f.HasJSON {
		t.Error("should detect JSON")
	}
	if !f.HasXML {
		t.Error("should detect XML")
	}
	if !f.HasBackgroundJobs {
		t.Error("should detect background jobs")
	}
	if !f.HasFileIO {
		t.Error("should detect file I/O")
	}
	if !f.HasBracedVars {
		t.Error("should detect braced vars")
	}
	if !f.HasFormatStrings {
		t.Error("should detect format strings")
	}
	if f.Complexity < 50 {
		t.Errorf("complexity should be >= 50, got %d", f.Complexity)
	}
	if f.RecommendedProfile == "" {
		t.Error("should have a recommended profile")
	}
	if f.RecommendedLevel < 1 || f.RecommendedLevel > 5 {
		t.Errorf("recommended level out of range: %d", f.RecommendedLevel)
	}
}

// TestFindMatchingBraceWithStrings ensures braces inside strings are ignored.
func TestFindMatchingBraceWithStrings(t *testing.T) {
	// Function with braces inside a string
	script := `function Foo { Write-Output "Hello { World }"; return 1 }`
	end := findMatchingBrace(script, 0)
	if end < 0 {
		t.Fatal("findMatchingBrace should find closing brace")
	}
	// The matched block should end at the function's closing brace, not the string's
	matched := script[:end]
	if !strings.Contains(matched, "return 1") {
		t.Error("matched block should contain 'return 1'")
	}
	if !strings.HasSuffix(strings.TrimSpace(matched), "}") {
		t.Error("matched block should end with }")
	}
}
