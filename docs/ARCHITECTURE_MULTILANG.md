# ObfusPS — Multi-Language Architecture

This document describes how **Go**, **PowerShell**, **Python**, and **C#** work together to maximize ObfusPS reliability, portability, and quality.

---

## Strategic Roles

| Language | Role | Strength |
|----------|------|----------|
| **Go** | Core engine, CLI, pipeline, packaging | Single binary, fast, cross-platform, no runtime deps |
| **PowerShell** | Native AST parser (`ast-parse.ps1`), validation runner | Uses `System.Management.Automation.Language` — gold standard for PS syntax |
| **C#** | Fallback AST parser (Windows) | Same parser as PowerShell; no pwsh required when .NET available |
| **Python** | GUI, test harness, CI, analysis | Rich ecosystem, tkinter, pytest, PyInstaller for standalone exe |

---

## 1. Go (Core Engine)

**Responsibilities:**
- Transform pipeline (iden, strenc, stringdict, numenc, cf, dead, frag)
- 5 obfuscation levels (char-join → Base64 → GZip → GZip+XOR+fragmentation)
- CLI parsing, profiles, validation orchestration
- Invokes AST helpers (PowerShell or C#) when `-use-ast` is set

**Principles:**
- Never embed PowerShell runtime; all execution lives in generated stubs
- UTF-8 with BOM + leading newline for correct Code Runner behavior
- Fallback to regex when AST unavailable

**Location:** `cmd/obfusps/`, `internal/engine/`, `pkg/obfusps/`

---

## 2. PowerShell (AST Helper)

**File:** `scripts/ast-parse.ps1`

**Purpose:**
- Uses `[System.Management.Automation.Language.Parser]::ParseInput`
- Extracts executable string spans (IEX, Add-Type, ScriptBlock::Create)
- Extracts exported functions (Export-ModuleMember -Function)
- Outputs JSON consumed by Go

**When used:** `-use-ast` + `-context-aware` or `-module-aware`, when pwsh/powershell is in PATH

**Fallback:** If PowerShell not found, Go tries C# parser (Windows) or regex

---

## 3. C# (AST Fallback for Windows)

**File:** `scripts/PSAstParser/PSAstParser.cs` (or `PSAstParser.exe` in build/)

**Purpose:**
- Same `System.Management.Automation.Language` as PowerShell
- Invoked when pwsh not found on Windows (e.g. build machines, CI)
- Outputs identical JSON format as ast-parse.ps1

**Build:**
```batch
# From project root (Windows)
build-PSAstParser.bat

# Or manually:
cd scripts/PSAstParser
dotnet publish -c Release -r win-x64 --self-contained false -o ../../build/PSAstParser
```
Requires .NET 8+ SDK (https://aka.ms/dotnet/download). Targets net8.0 with System.Management.Automation.

**When used:** `-use-ast` on Windows, when pwsh unavailable

---

## 4. Python (GUI + Testing)

### 4.1 ObfusPS-Tool.py

**Responsibilities:**
- File picker (tkinter), batch obfuscation
- Invokes Go engine with appropriate flags
- Auto-detects .psm1 → `-module-aware`
- Uses `-context-aware`, `-use-ast` when available
- Build: `pyinstaller ObfusPS-Tool.spec` → standalone exe

### 4.2 Test Harness

**File:** `tests/validate_regression.py`

**Responsibilities:**
- Run obfuscation on test corpus
- Validate outputs (original vs obfuscated)
- Metamorphic testing (reformat, re-obfuscate, compare)
- CI integration (exit code 0 = pass)

**Usage:**
```bash
python tests/validate_regression.py
python tests/validate_regression.py --corpus scripts/
```

---

## Environment

- **OBFUSPS_ROOT** — When set, used as base path to find `scripts/ast-parse.ps1` and `build/PSAstParser/`. Useful when running obfusps from a different directory.

---

## Quality Gates (Irreprochable)

| Gate | Implementation |
|------|----------------|
| **AST when possible** | PowerShell first, C# fallback (Windows), regex last |
| **Validation** | `-validate` compares stdout/stderr/exit code |
| **Reproducibility** | `-seed N` for deterministic builds |
| **Profiles** | safe, heavy, redteam — each tuned for use case |
| **Context-aware** | Skips strenc on IEX, Add-Type, ScriptBlock::Create |
| **Module-aware** | Protects Export-ModuleMember functions |
| **Regression tests** | Python harness + Go tests |
| **Documentation** | BEST_PRACTICES, ROADMAP, DOCUMENTATION |

---

## Invocation Order for AST

When `-use-ast` and (`-context-aware` or `-module-aware`):

1. **PowerShell** — `pwsh` or `powershell` + `scripts/ast-parse.ps1`
2. **C#** (Windows only) — `build/PSAstParser/PSAstParser.exe` or `scripts/PSAstParser/bin/Release/.../PSAstParser.exe`
3. **Regex** — Built-in Go regex (always available)

---

## Test Harness (Python)

**File:** `tests/validate_regression.py`

```bash
python tests/validate_regression.py                    # Obfuscate corpus, no validation
python tests/validate_regression.py --validate         # Obfuscate + validate outputs
python tests/validate_regression.py --corpus ./scripts # Custom corpus
python tests/validate_regression.py --no-ast           # Disable -use-ast
```

---

## References

- [BEST_PRACTICES.md](BEST_PRACTICES.md)
- [ROADMAP.md](ROADMAP.md)
- [DOCUMENTATION.md](DOCUMENTATION.md)
