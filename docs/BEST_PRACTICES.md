# ObfusPS — Best Practices and Research-Backed Techniques

This document consolidates **industry best practices**, **academic research**, and **practical recommendations** for reliable, semantic-preserving obfuscation. Follow these guidelines for optimal results across all script types.

---

## Golden Rules (Top 5)

1. **Always validate** — Use `-validate` for critical scripts; treat original as ground truth.
2. **Use context-aware** — Enable `-context-aware` when using IEX, Add-Type, ScriptBlock::Create.
3. **Use module-aware** — Enable `-module-aware` for `.psm1` modules with Export-ModuleMember.
4. **Pin your seed** — Use `-seed N` for reproducible builds and regression testing.
5. **Profile by use case** — `safe` for compatibility, `heavy/redteam` for labs; never skip testing.

---

## 0. ObfusPS Functionalities at a Glance

| Feature | Flag(s) | Best-practice use |
|---------|---------|-------------------|
| **Validation** | `-validate`, `-validate-args`, `-validate-stderr ignore`, `-validate-timeout N` | Compare outputs. Use `-validate-stderr ignore` when PowerShell profile pollutes stderr. |
| **Context-aware** | `-context-aware` | Skips strenc/stringdict for strings in IEX, Add-Type, ScriptBlock::Create. **Essential** for scripts with dynamic code. |
| **Module-aware** | `-module-aware` | Protects functions in Export-ModuleMember. **Mandatory** for modules. |
| **AST** | `-use-ast` | Uses native PowerShell AST for context/module parsing (pwsh required; fallback to regex). Best with `-context-aware` or `-module-aware`. |
| **Profiles** | `-profile safe\|heavy\|stealth\|paranoid\|redteam\|blueteam\|size\|dev` | `safe` = max compatibility; heavy/stealth/paranoid/redteam for Red Team; size for minimal output; dev for quick iteration. |
| **Reproducibility** | `-seed N` | Deterministic output for regression testing and CI. |
| **Report** | `-report` | Emits obfuscation report (techniques, complexity, sizes). Use to audit transforms. |
| **Dry-run** | `-dry-run` | Analyzes input without obfuscating. Validate script before full run. |
| **Docs** | `-docs`, `-examples` | Prints golden rules, recommended commands (copy-paste ready), and doc links. |
| **Anti-reverse** | `-anti-reverse` | Injects anti-debug checks. For lab/Red Team only; increases detection surface. |
| **Layers** | `-layers AST,Flow,Encoding,Runtime` | Override pipeline: AST=iden, Flow=cf/dead, Encoding=strenc/stringdict/numenc/fmt, Runtime=frag+level5. |
| **Logging** | `-log <file>` | RFC3339 logs: input, level, seed, output, errors. No impact on output. |
| **Levels** | `-level 1..5` | 1=char-join, 2–3=Base64, 4=GZip, 5=GZip+XOR+fragmentation+noise. |
| **Fragmentation** | `-minfrag`, `-maxfrag`, `-frag profile=tight\|medium\|loose\|pro` | Control fragment sizes; pro = stronger level 5. |
| **FlowSafeMode** | Default on | Skips cf/dead on try-catch/trap. Disable with `-flow-unsafe` (redteam/paranoid). |
| **No-integrity** | `-no-integrity` (default true) | Disables level 5 hash check; avoids empty output if file is re-saved. |
| **Fuzz** | `-fuzz N` | Generate N variants with different seeds. For detection testing. |

---

## 1. Semantic Preserving Transformations (SPT)

### Principles

**Semantic Preserving Transformations** alter syntactic structure while preserving functionality. Core categories:

| Category | Description | ObfusPS equivalent |
|----------|-------------|---------------------|
| **Layout** | Formatting, whitespace, newlines | `-fmt jitter` |
| **Data** | Storage patterns, string encoding, number masking | strenc (xor/rc4), stringdict, numenc |
| **Control flow** | Logical path restructuring | cf-opaque, cf-shuffle, dead |
| **Identifier renaming** | Replace names with arbitrary strings | `-iden obf` (iden transform) |

**Critical rule:** Incorrect transformations can silently alter functionality. Always validate.

### Pipeline mapping (explicit control)

```bash
-pipeline iden,strenc,stringdict,numenc,fmt,cf,dead,frag
-strenc xor|rc4 -strkey <hex>
-stringdict 0..100 -numenc
-cf-opaque -cf-shuffle -deadcode 0..100
```

---

## 2. Validation Strategy (Research-Backed)

### Differential testing

Treat the original script as ground truth. Run both with identical inputs; compare outputs. ObfusPS implements this via **`-validate`**.

```bash
obfusps -i script.ps1 -o out.ps1 -profile safe -validate
obfusps -i script.ps1 -o out.ps1 -profile safe -validate -validate-args "-Name foo -Count 5"
```

### Reproducibility for regression

Use **`-seed N`** for deterministic output. Re-run after obfuscator changes; diff outputs to detect regressions.

```bash
obfusps -i script.ps1 -o out.ps1 -seed 42 -profile safe
# After engine changes:
obfusps -i script.ps1 -o out2.ps1 -seed 42 -profile safe
diff out.ps1 out2.ps1  # or fc on Windows
```

### Metamorphic testing

Apply semantics-preserving changes to the original (e.g. add comments, reformat); obfuscate both variants; ensure behavior remains identical. Extends test coverage beyond a single script. (OBsmith, Auto-SPT)

### CI/CD integration

```bash
obfusps -i deploy.ps1 -o deploy-obf.ps1 -profile safe -seed 12345 -validate -q
# Exit code 0 = pass; non-zero = fail
```

---

## 3. AST-Based Approaches (Research)

### Why AST matters

- **PowerPeeler** (2024): Dynamic deobfuscation using AST nodes achieves ~95% correctness; AST correlation is key.
- **Invoke-Deobfuscation**: AST-based, semantics-preserving deobfuscation.
- **System.Management.Automation.Language.Parser**: Native PowerShell parser; gold standard for semantic accuracy.

### Current state

ObfusPS supports **`-use-ast`**: native PowerShell AST for IEX, Add-Type, ScriptBlock::Create, and Export-ModuleMember. Requires `pwsh` (or `powershell`) and `scripts/ast-parse.ps1`; falls back to regex if unavailable. See [ROADMAP.md](ROADMAP.md).

**Recommendation:** Use `-use-ast` with `-context-aware` or `-module-aware` for maximum accuracy on executable strings and module exports.

**Implication:** Test complex scripts thoroughly; prefer `-profile safe` or `-context-aware` when in doubt.

---

## 4. Context-Aware Obfuscation

### Principle

**Never encrypt or tokenize strings that are executed as code.** Distinguish:

- **Data strings** → safe to transform (strenc, stringdict)
- **Code strings** (IEX, Add-Type, ScriptBlock::Create, reflection) → skip strenc/stringdict

ObfusPS: use **`-context-aware`** to protect:

- `Invoke-Expression`, `iex`
- `[ScriptBlock]::Create`
- `Add-Type -TypeDefinition`, `Add-Type -MemberDefinition`

### Known gaps (use with caution)

- `[Reflection.Assembly]::Load*`
- `Add-Type -AssemblyName`
- Some `Invoke-Command -ScriptBlock` cases
- Embedded `powershell -Command "..."`

---

## 5. Module and Dot-Sourcing Awareness

### Principle

Protect **exported API** and **module boundaries**. Renaming exported functions breaks `Import-Module` consumers.

ObfusPS: use **`-module-aware`** to protect `Export-ModuleMember -Function A,B,C` names.

### Current scope

- Only `Export-ModuleMember -Function` is detected.
- `Import-Module`, dot-sourcing, module manifest not yet handled. Plan scripts accordingly.

---

## 6. Reserved Variables and Attributes

### Never rename (ObfusPS excludes these)

- Automatic variables: `$args`, `$_`, `$MyInvocation`, `$PSScriptRoot`, `$PSCommandPath`, `$null`, `$true`, `$false`, etc.
- Attributes: `[CmdletBinding()]`, `[Parameter()]`, `[ValidateSet()]`
- Reflection and dynamic types: names used by `GetType()`, `Add-Type`, etc.
- Prefix `$__` — always excluded from renaming (use for internal state).

---

## 7. Profile Selection by Use Case

| Use case | Profile | Rationale |
|----------|---------|-----------|
| **Maximum compatibility, identical result** | `safe` | Encoding only; no renaming; identical behavior to original |
| **CI/CD, regression** | `safe` + `-validate` | Guaranteed behavioral match |
| **Red Team / detection testing** | `heavy`, `stealth`, `paranoid`, `redteam` | Higher obfuscation; test before deployment; anti-reverse on redteam/paranoid |
| **Blue Team / defensive testing** | `blueteam` | Level 5, no anti-reverse, deterministic |
| **Size optimization** | `size` | Minimal pipeline; level 4 |
| **Quick iteration** | `dev` | Level 2, iden only |
| **Light obfuscation** | `light` | iden, stringdict, numenc, frag; level 1–5 |

---

## 8. Path and Closure Safety

### Path fallback

Obfuscated scripts run in a scriptblock; `$MyInvocation.MyCommand.Path` and `$PSScriptRoot` may be **empty**. Provide a fallback:

```powershell
$script:RootPath = try {
    $p = $MyInvocation.MyCommand.Path
    if ($p) { Split-Path -Parent $p } else { (Get-Location).Path }
} catch { (Get-Location).Path }
```

### Closures

Use **`$script:variable`** instead of local variables for shared state; closures behave correctly in scriptblock context.

---

## 9. Encoding and Output

- **Input:** UTF-8 (with or without BOM)
- **Output:** UTF-8 with BOM + leading newline (preserves characters; newline avoids BOM breaking first token in Code Runner / some hosts)
- **Do not re-save** obfuscated files in ANSI/UTF-16; Base64 and embedded strings can corrupt
- **`-no-integrity`** (default true): level 5 skips integrity hash; avoids empty output if file is re-saved in another editor

---

## 10. Pre-Obfuscation Checklist

- [ ] Script runs correctly on target PowerShell (5.1 / 7.x)
- [ ] Path fallback for `$PSScriptRoot` / `$MyInvocation.MyCommand.Path` if needed
- [ ] Closures use `$script:` for shared state
- [ ] No hardcoded paths or environment-specific assumptions (or document them)
- [ ] If module: `Export-ModuleMember -Function` present; use `-module-aware`
- [ ] If IEX/Add-Type/ScriptBlock::Create: use `-context-aware`

---

## 11. Post-Obfuscation Checklist

- [ ] Run `-validate` (or manual comparison) with same args
- [ ] Test on PowerShell 5.1 and 7.x if using Unicode/ANSI
- [ ] Verify `$PSScriptRoot` / path-dependent logic
- [ ] Do not re-save in another encoding; keep UTF-8 BOM

---

## 12. Common Pitfalls

| Pitfall | Mitigation |
|---------|------------|
| **strenc breaks IEX/Add-Type strings** | Use `-context-aware` |
| **Renamed exported functions break Import-Module** | Use `-module-aware` |
| **Empty $PSScriptRoot** | Provide path fallback (§8) |
| **Closures fail in scriptblock** | Use `$script:variable` |
| **Different output after obfuscation** | Use `-profile safe`; validate with `-validate` |
| **Re-saved file produces empty output (level 5)** | Keep `-no-integrity` true; do not re-save |
| **Variable interpolation in strings** | strenc/stringdict can break `"Result:$x"`; use `-profile safe` or avoid strenc on those strings |

---

## 13. Pro Tips

- **Combine context + module awareness:** `-context-aware -module-aware` for complex scripts
- **Audit before obfuscating:** `obfusps -i script.ps1 -dry-run -report`
- **Fuzz variants for detection testing:** `obfusps -i payload.ps1 -o out -fuzz 5` (generates 5 variants)
- **Layers for fine control:** `-layers Encoding,Runtime` for encoding + level 5 without flow/iden
- **Log for debugging:** `-log obfusps.log` to trace seed, level, errors

---

## 14. Recommended Workflow for Critical Scripts

1. **Obfuscate** with `-profile safe` for maximum compatibility
2. **Validate** with `-validate` and `-validate-args` if the script accepts parameters
3. **Pin seed** with `-seed N` for reproducible builds
4. **Test** on PowerShell 5.1 and 7.x if using Unicode or ANSI
5. **Document** any known limitations for your script (modules, classes, etc.)

---

## 15. Example Commands by Scenario

### CI/CD pipeline (reproducible + validated)

```bash
obfusps -i deploy.ps1 -o deploy-obf.ps1 -profile safe -seed 12345 -validate
```

### Module (protect exported API)

```bash
obfusps -i MyModule.psm1 -o MyModule-obf.psm1 -profile balanced -module-aware
```

### Script with IEX / Add-Type (protect executable strings)

```bash
obfusps -i tool.ps1 -o tool-obf.ps1 -profile heavy -context-aware -use-ast
```

### Complex script (context + module)

```bash
obfusps -i Tool.ps1 -o Tool-obf.ps1 -profile heavy -context-aware -module-aware -use-ast -validate
```

### Red Team / detection testing

```bash
obfusps -i payload.ps1 -o payload-obf.ps1 -level 5 -profile redteam -anti-reverse -report
```

### Audit without obfuscating

```bash
obfusps -i script.ps1 -dry-run -report
```

### Minimal size, level 4

```bash
obfusps -i script.ps1 -o out.ps1 -profile size -level 4
```

### Fuzz 5 variants (unique seeds)

```bash
obfusps -i payload.ps1 -o variant -fuzz 5
# Produces variant-0.ps1 .. variant-4.ps1
```

---

## References

- **PowerPeeler:** Precise and General Dynamic Deobfuscation for PowerShell (2024)
- **Invoke-Deobfuscation:** AST-Based Semantics-Preserving Deobfuscation (IEEE)
- **OBsmith:** LLM-Powered Obfuscator Testing (differential and metamorphic validation)
- **Auto-SPT:** Semantic Preserving Transformations for Code
- [System.Management.Automation.Language.Parser](https://learn.microsoft.com/en-us/dotnet/api/system.management.automation.language.parser) (Microsoft)
- [DOCUMENTATION.md](DOCUMENTATION.md) — techniques, reserved vars, FlowSafeMode
- [ROADMAP.md](ROADMAP.md) — AST, module-aware, context-aware roadmap
