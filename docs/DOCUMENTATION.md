# ObfusPS Technical Documentation

> **Roadmap:** See [ROADMAP.md](ROADMAP.md) for planned improvements. **Best practices:** See [BEST_PRACTICES.md](BEST_PRACTICES.md) for research-backed techniques.

## 0. Complete techniques and technologies

### Encoding levels (packers)

| Level | Technique | Technologies |
|-------|-----------|--------------|
| **1** | Script → character codepoint array | `[char]`, `[char]::ConvertFromUtf32` (emojis U+FFFF+), UTF-8 console |
| **2** | Base64(UTF-8) | `[Convert]::FromBase64String`, `[Text.Encoding]::UTF8.GetString` |
| **3** | Base64 + intermediate variable | Same as 2 with 2-step decode |
| **4** | GZip → Base64 | `IO.MemoryStream`, `IO.Compression.GzipStream`, `IO.StreamReader` |
| **5** | GZip + XOR + Base64 + fragmentation + shuffled order | LCG (32-byte key), variable fragmentation, GetMethod(FromBase64String), opaque predicates, noise |

### Pipeline transforms

| Transform | Technique | Description |
|-----------|-----------|-------------|
| **iden** | Identifier renaming | Variables `$var` and function names renamed; reserved variables excluded |
| **strenc** | String encryption | XOR or RC4 (256-byte S-box); inline or helper decryption |
| **stringdict** | Tokenization | Long strings → `$D[i]+$D[j]+...` with dictionary |
| **numenc** | Number encoding | `42` → `((0x2A -bxor 0x1234)-1)` (arithmetic/bitwise) |
| **fmt** | Format jitter | Random spaces, newlines |
| **cf-opaque** | Opaque predicate | Wrapper `if(1 -eq 1){ ... }` |
| **cf-shuffle** | Block shuffling | Function block reordering |
| **dead** | Dead code | Never-taken branches, unused variables, dummy functions |
| **anti-reverse** | Anti-debug | `[System.Diagnostics.Debugger]::IsAttached` → exit if debugger |

### Technologies used

| Technology | Role |
|------------|------|
| **Go 1.24+** | Engine (parsing, transforms, packers, RNG, I/O) |
| **SHA-256** | Seed derivation (first 8 bytes); integrity hash (4 bytes, level 5) |
| **LCG (Linear Congruential Generator)** | `seed * 1103515245 + 12345` — 32-byte XOR key derivation |
| **GZip** | Compression (levels 4–5) |
| **Base64** | Payload encoding; fragment order (1 byte/index, max 256) |
| **XOR** | Byte-wise encryption (level 5) |
| **RC4** | String encryption (256-byte S-box, keystream XOR) — *obfuscation only* |
| **Fisher-Yates** | Fragment and order shuffling |
| **Regex** | Variable `$var`, function `function Name(`, string detection |
| **Python 3** | Optional GUI (tkinter, colorama) |
| **PowerShell .NET** | `Convert`, `Encoding`, `Stream`, `GzipStream`, `Debugger` |

### Level 5 — details

- **Fragmentation**: variable size (minFrag–maxFrag), max 256 fragments
- **Order**: shuffled index array, Base64-encoded
- **Noise**: dead computation blocks, `if(0 -eq 1)` branches, overwritten variables
- **Opaque predicates**: `if(1 -eq 1)`, `($v=1) -eq 1`, etc.
- **Structural polymorphism**: linear template, nested scriptblock, or intermediate variables
- **Decoy**: fake seed, unused variable
- **GetMethod**: `[Convert].GetMethod(FromBase64String,...).Invoke` — avoids clear-text signature

### Reserved variables (never renamed)

`$args`, `$input`, `$null`, `$true`, `$false`, `$error`, `$foreach`, `$?`, `$^`, `$_`, `$host`, `$pid`, `$pwd`, `$pshome`, `$psversiontable`, `$psboundparameters`, `$myinvocation`, `$pscmdlet`, `$psscriptroot`, `$pscommandpath`, `$lastexitcode`, `$ofs`, `$stacktrace`, `$sender`, `$eventargs`, `$event`, `$nestedpromptlevel`, `$matches`, `$consolefilename`, `$shellid`, `$executioncontext`, `$this`, `$isglobal`, `$isscript`. Prefix `$__` excluded.

### FlowSafeMode

When enabled (default): no CF-opaque, CF-shuffle, deadcode on try-catch/trap blocks. Disabled with `-flow-unsafe` (redteam/paranoid) or redteam/paranoid profiles.

---

## 1. Randomness governance (quality & reproducibility)

### Deterministic mode
- **`-seed <integer>`**: uses a fixed seed. Same input + same seed → **same stub**.
- Use for: tests, A/B comparisons, regressions, debugging.

### Random mode (default)
- When **`-seed` is not set**, the seed is derived from `hash(script_content) ^ random`: same script → stable base, unique output per run. The effective seed is **written in the generated script** in the footer: `# ObfusPS | seed=1234567890`.
- In non-quiet mode, the seed is also printed on stderr: `Seed: 12345 (re-run with -seed 12345 for same output)`.
- **Benefit**: a posteriori reproducibility (you can re-obfuscate identically with `-seed 12345`).

---

## 2. Obfuscation levels: goals and limits

| Level | Goal | Limits |
|-------|------|--------|
| 1 | Script as character code array + IEX | Trivial to reconstruct (concat + eval). |
| 2–3 | Base64 + UTF8 decode + execution | Easy static detection (Base64 + IEX signature). |
| 4 | GZip + Base64 + decompression + execution | Typical .NET chain (GzipStream, etc.) profiled by AV/AMSI. |
| 5 | GZip + XOR (32-byte LCG-derived key) + Base64 + fragmentation + shuffled order + stronger noise | Without execution: need seed, fragment order, un-XOR, decompress. Does not claim to be "unbreakable". |

**What the tool does not claim to do**
- Make the script unbreakable to a determined attacker.
- Reliably bypass AMSI/Defender (behavioral signatures and .NET types remain detectable).
- Guarantee no regression on every PowerShell script (PS7+ syntax, external modules, etc.).

### Reliability and "pro" usage

- **Level 5 by default**: integrity check (hash) is **disabled** (`-no-integrity` by default) to avoid empty output if the generated file is re-saved. To re-enable: `-no-integrity=false`.
- **Fragmentation profile**: `-frag profile=pro` (level 5) uses smaller fragments (5–14 characters). Others: `tight`, `medium`, `loose`. For **large scripts** (e.g. 2000+ lines), the engine automatically increases fragment size so the fragment count never exceeds 256; no need to change profile or options.
- **Input**: the file must be **valid UTF-8**. No size limit (large scripts are supported). Otherwise an explicit error message is shown.
- **Level 1**: emojis / codepoints > U+FFFF handled via `[char]::ConvertFromUtf32`.

### Preserving identical results

- **Profile `safe`**: encoding only (Base64, level 3), no variable/function renaming, no flow transforms. Use for maximum compatibility: **results remain identical** to the original script.
- **Path fallback**: do not depend execution on file path; after obfuscation the script runs in a scriptblock; `$MyInvocation.MyCommand.Path` and `$PSScriptRoot` may be empty. Provide a fallback (e.g. `(Get-Location).Path`) instead of `exit` or error.

  **Reusable snippet (Path fallback)** — paste at the top of your script if you use the path:

  ```powershell
  $scriptPath = $MyInvocation.MyCommand.Path
  if (-not $scriptPath) { $scriptPath = $null }  # execution from scriptblock (obfuscated) or piped
  # Option: neutral key for logic that requires a value
  $key = if ($scriptPath) { [System.IO.Path]::GetFileName($scriptPath) } else { 0 }
  ```
- **Avoid PS7+‑only syntax** if target is PS 5.1 (e.g. ternary operator `? :`).
- **Test the obfuscated script** after generation (same input/output as original) to detect regressions.

### Reading / writing obfuscated files

- **Output**: the tool always writes **UTF-8 with BOM** plus a leading newline so Windows/PowerShell open the script as UTF-8 and avoid read errors. The newline prevents the BOM from breaking the first token (e.g. `[Console]::OutputEncoding`) in Code Runner and similar contexts.
- **Do not re-save the obfuscated file** in another encoding (e.g. ANSI, UTF-16): this can corrupt Base64 strings and cause execution to fail. Keep **UTF-8 (with BOM)** if you edit the file.
- **Input**: provide source scripts in **UTF-8** (with or without BOM) so obfuscation does not degrade special characters or emojis.

---

## 3. Legitimate use cases

- **Intellectual property protection**: make copying internal scripts harder.
- **Anti-copy / licensing**: make static analysis more costly.
- **Authorized labs and Red Team**: detection testing, signature evolution.
- **Regression testing**: with `-seed` to compare outputs before/after obfuscator changes.

---

## 4. Metrics (objective)

In non-quiet mode, the following metrics are printed on stderr after generation:

- **size**: generated script size in bytes.
- **unique**: number of unique symbols (runes).
- **entropy**: approximate entropy (bits per symbol).
- **alnum_ratio**: alphanumeric characters ratio / total (0–1).

Example:  
`Metrics: size=12345 bytes | unique=87 | entropy=4.52 | alnum_ratio=0.71`

---

## 5. Engine vs stub architecture

**The Go engine never embeds PowerShell logic directly; all runtime behavior lives in generated stubs.**

```
┌─────────────────────────────────────────────────────────────────┐
│  ENGINE (Go)                                                     │
│  - Reads input script (UTF-8)                                    │
│  - Applies transforms (iden, strenc, stringdict, etc.)           │
│  - Encodes payload (Base64, GZip, XOR, fragmentation)            │
│  - Generates stub (PowerShell)                                   │
│  - Writes output file                                            │
│  → No embedded PowerShell; engine is pure transformation         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STUB (generated .ps1)                                           │
│  - Decodes payload at runtime                                    │
│  - Invokes scriptblock with decoded script                       │
│  - All runtime behavior is in the stub; engine never executes PS │
└─────────────────────────────────────────────────────────────────┘
```

Important for audits: the engine produces text; it does not interpret or execute PowerShell. Stubs are self-contained and deterministic given the same input and seed.

---

## 6. Internal architecture (summary)

- **Generation**: `obfuscate()` (levels.go) produces the stub by level.
- **Transformation**: optional pipeline (identifier morphing, string dict, strenc, etc.) applied before the packer.
- **Assembly**: runner concatenates payload + signature + seed in the footer.
- **RNG**: `InitRNG()` (random.go) handles deterministic vs random mode and writes the seed used. Without `-seed`, the seed is derived from the script hash.
- **Log**: option `-log <file>` (disabled by default) writes a minimal log (input, level, seed, output, errors) for debugging.

---

## 7. Threat model (explicit)

ObfusPS describes techniques; this section makes the **threat model** explicit.

| Aspect | Assumption |
|--------|------------|
| **Adversary** | Static analysis, signature-based detection, casual reverse engineering. Not a determined attacker with dynamic analysis and full tooling. |
| **Protection goal** | Hinder static extraction of logic; make signature matching harder. Not cryptographic or unbreakable. |
| **Trust boundary** | Input script is trusted. Output stub is run in user's environment. Engine does not call external services. |
| **AMSI/Defender** | Tool does not implement bypass. Encoding patterns may still be profiled. Behavioral detection remains possible. |
| **Semantic correctness** | Best-effort with regex; not provable without AST. Use `-validate` to verify behavior. |

👉 Optional but useful for audits and compliance: clarify what the tool protects against and what it does not.

---

## 8. Critical policies and limits (advanced scripts)

These rules describe what the obfuscator avoids modifying to limit regressions. Without native AST, some limits are not automatically detected.

### 8.1 Deterministic randomization
- **`-seed N`**: same input + same options → identical output. Essential for CI, audit, debug.
- Command: `obfusps -i script.ps1 -o out.ps1 -level 5 -seed 42`

### 8.2 Dynamic types
| Element | Rule |
|---------|------|
| `.GetType()` | Do not modify what affects the type |
| Reflection | Never obfuscate names used by reflection |
| `Add-Type` | Never touch embedded C# code |
| `[Type]::Member` | Never rename what is used here |

### 8.3 Executable-semantic strings
Strings in `Invoke-Expression`, `ScriptBlock::Create()`, `Add-Type`, `iex`, `&` = code, not data. With `-strenc`, test or use `-strenc off`.

### 8.4 Attributes and metadata
`[CmdletBinding()]`, `[Parameter()]`, `[ValidateSet()]`: read-only, never obfuscated.

### 8.5 Safe profile
- **`-profile safe`**: maximum compatibility — iden only, level 3, no strenc/stringdict/numenc/cf/dead. To guarantee identical result.

### 8.6 Modules and dot-sourcing
Do not rename what crosses `. .\lib.ps1` or `Import-Module`. Test modular scripts.

---

## 9. Minor caveats

- **Never re-enable file integrity by default** (even level 5+): keep `-no-integrity` by default to avoid empty output if the obfuscated file is re-saved or encoding changes.
- **Always provide a fallback**: when running from a scriptblock (obfuscated), `MyCommand.Path` is empty. Use `MyCommand.Definition` or a neutral key (e.g. `$key = 0`) so the script works after obfuscation too.
- **Do not overdo noise**: too much noise = slowness and bug risk (variables, StrictMode). Keep noise blocks moderate at level 5.
- **Test on PS 5.1 and 7.x**: validate scripts (especially emojis and ANSI sequences) on both versions to avoid regressions and mojibake.

---

## 10. Automated tests

- **`go test ./internal/engine/...`**: unit tests (levels 1–5, determinism with seed, seed presence in output).
- Manually test on **PowerShell 5.1 and 7.x** (emojis, ANSI) if target scripts use Unicode or colors.
- To compare original vs obfuscated output for a given script: run both with the same parameters and compare outputs (excluding timestamps / env noise).
