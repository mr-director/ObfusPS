# ObfusPS AST Helper — uses native PowerShell Parser to extract executable strings and exported functions.
# Outputs JSON to stdout for consumption by the Go engine.
# Usage: Get-Content script.ps1 | pwsh -File ast-parse.ps1
# Or: pwsh -File ast-parse.ps1 -InputScript (Get-Content script.ps1 -Raw)

param([string]$InputScript)

$ErrorActionPreference = 'Stop'
$script = if ($InputScript) { $InputScript } else { try { [System.Console]::In.ReadToEnd() } catch {} }
if (-not $script -and $input) { $script = $input | Out-String }

if ([string]::IsNullOrWhiteSpace($script)) {
    Write-Output '{"executableStrings":[],"exportedFunctions":[],"error":"empty input"}'
    exit 0
}

$executableSpans = [System.Collections.Generic.List[object]]::new()
$exportedFunctions = [System.Collections.Generic.HashSet[string]]::new()

try {
    $tokens = $null
    $errors = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseInput($script, [ref]$tokens, [ref]$errors)

    if ($null -eq $ast) {
        Write-Output ('{"executableStrings":[],"exportedFunctions":[],"error":"parse failed"}')
        exit 0
    }

    # Find executable string spans (IEX, ScriptBlock::Create, Add-Type -TypeDefinition/-MemberDefinition)
    $ast.FindAll({ $true }, $true) | ForEach-Object {
        $node = $_
        if ($node -is [System.Management.Automation.Language.CommandAst]) {
            $cmdName = $node.GetCommandName()
            if ($cmdName -match '^(Invoke-Expression|iex)$') {
                foreach ($elem in $node.CommandElements) {
                    if ($elem -is [System.Management.Automation.Language.StringConstantExpressionAst] -or $elem -is [System.Management.Automation.Language.ExpandableStringExpressionAst]) {
                        $executableSpans.Add(@($elem.Extent.StartOffset, $elem.Extent.EndOffset))
                        break
                    }
                }
            }
            if ($cmdName -eq 'Add-Type') {
                $idx = 0
                $elems = @($node.CommandElements)
                for ($i = 0; $i -lt $elems.Count; $i++) {
                    $e = $elems[$i]
                    $val = if ($e -is [System.Management.Automation.Language.CommandParameterAst]) { $e.ParameterName }
                           elseif ($e.Value) { $e.Value }
                    if ($val -in '-TypeDefinition', '-MemberDefinition' -and ($i + 1) -lt $elems.Count) {
                        $next = $elems[$i + 1]
                        if ($next -is [System.Management.Automation.Language.StringConstantExpressionAst] -or $next -is [System.Management.Automation.Language.ExpandableStringExpressionAst]) {
                            $executableSpans.Add(@($next.Extent.StartOffset, $next.Extent.EndOffset))
                        }
                    }
                }
            }
        }
        if ($node -is [System.Management.Automation.Language.InvokeMemberExpressionAst]) {
            $target = $node.Target
            $member = $node.Member
            if ($target -and $member -and $member.Name -eq 'Create') {
                $typeStr = $target.Type.ToString()
                if ($typeStr -match 'ScriptBlock') {
                    foreach ($arg in $node.Arguments) {
                        if ($arg -is [System.Management.Automation.Language.StringConstantExpressionAst] -or $arg -is [System.Management.Automation.Language.ExpandableStringExpressionAst]) {
                            $executableSpans.Add(@($arg.Extent.StartOffset, $arg.Extent.EndOffset))
                            break
                        }
                    }
                }
            }
        }
    }

    # Find Export-ModuleMember -Function A,B,C
    $ast.FindAll({ $args[0] -is [System.Management.Automation.Language.CommandAst] }, $true) | ForEach-Object {
        $cmd = $_
        if ($cmd.GetCommandName() -eq 'Export-ModuleMember') {
            $elems = @($cmd.CommandElements)
            for ($i = 0; $i -lt $elems.Count; $i++) {
                $e = $elems[$i]
                if ($e -is [System.Management.Automation.Language.CommandParameterAst] -and $e.ParameterName -eq 'Function') {
                    if (($i + 1) -lt $elems.Count) {
                        $next = $elems[$i + 1]
                        if ($next -is [System.Management.Automation.Language.ArrayLiteralAst]) {
                            $next.Elements | Where-Object { $_ -is [System.Management.Automation.Language.StringConstantExpressionAst] } | ForEach-Object { if ($_.Value) { [void]$exportedFunctions.Add($_.Value.Trim()) } }
                        } else {
                            $val = if ($next -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $next.Value } elseif ($next.Value) { $next.Value } else { $next.Extent.Text }
                            foreach ($part in ($val -split '[,\s;]+')) { $p = $part.Trim(); if ($p) { [void]$exportedFunctions.Add($p) } }
                        }
                    }
                    break
                }
            }
        }
    }

} catch {
    $errMsg = ($_.Exception.Message -replace '\\','\\\\' -replace '"','\"' -replace "`n",'\n' -replace "`r",'\r' -replace "`t",'\t')
    Write-Output ('{"executableStrings":[],"exportedFunctions":[],"error":"' + $errMsg + '"}')
    exit 0
}

$result = @{ executableStrings = @($executableSpans); exportedFunctions = @($exportedFunctions) }
$result | ConvertTo-Json -Compress | Write-Output
