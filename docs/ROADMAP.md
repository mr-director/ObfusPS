# ObfusPS Roadmap

This document describes the **strategic vision** and planned improvements. Current implementation status is noted where relevant.

---

## Target objectives

| Objective | Description | Current status |
|-----------|-------------|----------------|
| **0 anti-debug** | No `[Debugger]::IsAttached` injection | `-anti-reverse` optional (redteam/paranoid); `-profile safe` has none |
| **0 suspicious behavior** | Avoid malware-typical patterns | Profile-dependent |
| **0 useless noise** | No dead code / opaque predicates when not needed | Level 5 adds noise; `-profile safe` has none |
| **AMSI-friendly** | No AMSI bypass; minimal detection surface | Encoding-based; no explicit AMSI manipulation |
| **Deterministic** | `-seed N` → identical output | ✅ Implemented |
| **AST-only + clean encoding** | Semantic parsing + minimal encoding | ❌ Regex-based today |

---

## 1. `-module-aware` mode (implemented: base)

**Status:** Base implementation. Exported functions (Export-ModuleMember -Function) are protected from renaming.

### Previous limitation

ObfusPS treats each file in isolation. Scripts that rely on:

- `$PSScriptRoot` / `$PSCommandPath`
- Dot-sourcing (`. .\module.ps1`)
- `Import-Module` with relative paths
- Exported functions / module manifest

…can break or behave incorrectly after obfuscation.

### Implemented

- Detects `Export-ModuleMember -Function A,B,C` and protects those function names from renaming
- Use `-module-aware` flag

### Future

- Detect `Import-Module` and dot-sourcing
- Respect full module boundaries
- Preserve paths and references across related files

**Note:** No mainstream open-source PowerShell obfuscator handles modules and dot-sourcing well; this is a step toward that goal.

---

## 2. Context-aware obfuscation (implemented)

**Status:** Implemented. Use `-context-aware` to skip strenc and stringdict for strings in IEX, Add-Type, ScriptBlock::Create.

### Previous limitation

All strings are treated the same. The engine does not distinguish:

- **Data strings** (safe to encrypt)
- **Code strings** (e.g. in `Invoke-Expression`, `Add-Type`, `ScriptBlock::Create`, reflection)

Encrypting code strings breaks execution. See [DOCUMENTATION.md §8.3](DOCUMENTATION.md#83-executable-semantic-strings).

### Implemented

- Detect strings used as **executable context** (Invoke-Expression, iex, ScriptBlock::Create, Add-Type -TypeDefinition/-MemberDefinition)
- Skip strenc and stringdict for those strings
- Use `-context-aware` flag

### Future refinements

- `[Reflection.Assembly]::Load*`
- `Add-Type -AssemblyName`
- `Invoke-Command -ScriptBlock {}` (some cases)
- Embedded `powershell -Command "..."`

---

## 3. `-validate` mode (implemented)

**Status:** Implemented. After obfuscation, runs original and obfuscated scripts and compares stdout, stderr, exit code.

### Goal

Verify that the obfuscated script has the **same runtime behavior** as the original:

- Run original and obfuscated with the same inputs
- Compare outputs (exit code, stdout, stderr)
- Report regressions

Use: `obfusps -i script.ps1 -o out.ps1 -validate [-validate-args "-Name x"]`

### Use cases

- CI/CD checks
- Regression tests
- Confidence before deployment

---

## 4. Native PowerShell AST (blocker #1 — the only real "hard gap")

### Why it remains a gap

- **Regex-based renaming → not provable.** Without a symbol graph, you cannot formally prove that renaming preserves semantics. Collisions and shadowing cannot be ruled out.
- **No symbol graph.** The engine has no representation of scopes, bindings, or references. It treats tokens in isolation.
- **All regex-based obfuscators hit a ceiling.**

### Current state

Solid regex patterns and many safeguards. Still **fragile** on:

- Complex scopes
- Closures
- Classes
- Advanced attributes
- `dynamicparam`, variable shadowing

Current parsing (`internal/engine/types.go`): regex for `$var`, `function Name(`, strings, etc. No semantic representation — cannot reliably distinguish variable vs literal, code vs data, or module boundaries.

### What native AST would deliver

- **Real scope awareness** — collision-free renaming across nested scopes
- **Reliable detection** of executable contexts (IEX, Add-Type, ScriptBlock::Create, reflection)
- **Proper support** for full modules (Import-Module, dot-sourcing, manifest)
- **Semantic guarantees** for closures, classes, advanced attributes

👉 **This is the only true "game changer" evolution left.**

### Technical direction

Use **`System.Management.Automation.Language.Parser`** to obtain: variables vs literals, code vs data, module boundaries, precise scope. Options: hosted PowerShell runtime, or a Go library that parses PowerShell (if mature and maintained).

---

## Technical assessment and honesty

### 1. Native AST — still the #1 blocker

**Current state:** Advanced regex, many safeguards, but no real AST.

**Why it still blocks:** Even with -context-aware, -module-aware, FlowSafeMode, and smart exclusions, you cannot guarantee 100% semantic identity for all PowerShell scripts.

**Examples that cannot be guaranteed without AST:**
- Complex closures
- PowerShell classes
- dynamicparam blocks
- Nested scopes and variable shadowing
- Cmdlets with advanced metadata

**Conclusion:** Until native AST exists, remain honest (as the docs do): *"Reliable for standard and advanced scripts, not universal."* That's acceptable — but it's the last technical wall.

---

### 2. Validation — enhancements (optional, highly valued in CI)

`-validate` is correct. To make it **excellent**:

| Enhancement | Description |
|-------------|-------------|
| **-validate-mode strict/lenient** | strict = exact byte-for-byte comparison; lenient = ignore timestamps, GUIDs, random outputs, minor whitespace |
| **-validate-hash** | Logical hash of outputs instead of raw comparison — less sensitive to noise (e.g. `Get-Date`, `[guid]::NewGuid()`) |
| **-validate-json** | JSON output (stdout, stderr, exit code, duration) for CI pipelines |
| **Explicit diff on failure** | Show clear line-by-line diff when validation fails |

👉 Not mandatory, but highly valued in advanced CI workflows.

---

### 3. Context-aware — incomplete but well started

**Covered:** IEX/iex, ScriptBlock::Create, Add-Type (-TypeDefinition/-MemberDefinition)

**Still missing (refinements, not defects):**
- `[Reflection.Assembly]::Load*`
- `Add-Type -AssemblyName`
- `Invoke-Command -ScriptBlock {}` in some cases
- Embedded `powershell -Command "..."`

---

### 4. Module-aware — solid base, not yet complete

**Implemented:** Export-ModuleMember protected ✅

**Missing for full support:**
- Import-Module detection
- Dot-sourcing (`. .\file.ps1`)
- Multi-file handling

Since this is not promised, it is acceptable as-is.

---

### 5. AMSI / Defender awareness (not implemented)

- **Current position:** ObfusPS does not implement AMSI/Defender bypass; encoding only, minimal detection surface. Tool remains research- and authorized-testing oriented.
- **Possible awareness:** Detect and flag (or exclude) sensitive strings (AMSI bypass patterns, etc.) to avoid typical malware signatures. Primarily for risk and limitation documentation, not for offensive obfuscation.

---

### 6. Pipeline .ps1 → .exe (absent)

Many users want script-to-exe packaging (e.g. PowerCrypt). ObfusPS has the architecture to do it cleanly. As long as it is not claimed, it is not a defect.

---

## Priority order (proposed)

1. **Native AST** — foundation for all semantic features
2. **Context-aware obfuscation** — fix strenc/string handling
3. **`-module-aware`** — support modules and dot-sourcing
4. **`-validate`** — execution comparison for regression testing

---

## Implementation guide for optimal results

### Recommended command patterns

| Scenario | Command | Rationale |
|----------|---------|-----------|
| **Production / CI** | `obfusps -i script.ps1 -o out.ps1 -profile safe -seed N -validate` | Identical behavior, reproducible, validated |
| **Modules (.psm1)** | `obfusps -i MyModule.psm1 -o out.psm1 -profile balanced -module-aware` | Protects exported API |
| **Dynamic code (IEX/Add-Type)** | `obfusps -i tool.ps1 -o out.ps1 -profile heavy -context-aware` | Skips strenc on executable strings |
| **Red Team / detection** | `obfusps -i payload.ps1 -o out.ps1 -level 5 -profile redteam -report` | Maximum obfuscation, audit report |
| **Regression testing** | `obfusps -i script.ps1 -o out.ps1 -seed 42 -profile safe` then `diff` outputs | Detect engine regressions |

### Quality gates

1. **Always run `-validate`** before deploying obfuscated scripts to production.
2. **Use `-seed N`** for reproducible builds in CI; diff outputs after engine changes.
3. **Combine `-context-aware` and `-module-aware`** when script uses both dynamic code and modules.
4. **Test on target runtimes** — PowerShell 5.1 and 7.x if Unicode or encoding-dependent.

---

## References

- [BEST_PRACTICES.md](BEST_PRACTICES.md) — research-backed techniques, SPT, validation, AST
- [DOCUMENTATION.md](DOCUMENTATION.md) — current techniques, limits, reserved variables
- [README.md](../README.md) — usage, profiles, options
- §8 Critical policies (DOCUMENTATION) — dynamic types, IEX, dot-sourcing
