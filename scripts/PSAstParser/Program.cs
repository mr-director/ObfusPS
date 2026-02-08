// PSAstParser — ObfusPS AST fallback for Windows when pwsh is unavailable.
// Uses System.Management.Automation.Language (same as PowerShell).
// Outputs JSON identical to scripts/ast-parse.ps1 for consumption by Go engine.
// Usage: script content piped to stdin, or: PSAstParser.exe -InputScript "script"
// Go invokes with Stdin for large scripts (avoids command-line limits).

using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Management.Automation.Language;
using System.Text;

string script;
if (args.Length >= 2 && args[0].Equals("-InputScript", StringComparison.OrdinalIgnoreCase))
    script = args[1];
else if (args.Length > 0 && !args[0].StartsWith("-"))
    script = string.Join(" ", args);
else
    script = Console.In.ReadToEnd();

if (string.IsNullOrWhiteSpace(script))
{
    Emit(new Result { Error = "empty input" });
    return 0;
}

var result = Parse(script);
Emit(result);
return 0;

static Result Parse(string script)
{
    try
    {
        var tokens = Array.Empty<Token>();
        var errors = Array.Empty<ParseError>();
        var ast = Parser.ParseInput(script, out tokens, out errors);
        if (ast == null)
            return new Result { Error = "parse failed" };

        var executableSpans = new List<int[]>();
        var exportedFunctions = new HashSet<string>(StringComparer.OrdinalIgnoreCase);

        foreach (var node in ast.FindAll(_ => true, searchNestedScriptBlocks: true))
        {
            if (node is CommandAst cmd)
            {
                var name = cmd.GetCommandName();
                if (name != null && (name.Equals("Invoke-Expression", StringComparison.OrdinalIgnoreCase) || name.Equals("iex", StringComparison.OrdinalIgnoreCase)))
                {
                    foreach (var elem in cmd.CommandElements)
                    {
                        if (elem is StringConstantExpressionAst str)
                        {
                            executableSpans.Add(new[] { str.Extent.StartOffset, str.Extent.EndOffset });
                            break;
                        }
                    }
                }
                if (name != null && name.Equals("Add-Type", StringComparison.OrdinalIgnoreCase))
                {
                    var elems = cmd.CommandElements.ToArray();
                    for (int i = 0; i < elems.Length; i++)
                    {
                        var paramName = (elems[i] as CommandParameterAst)?.ParameterName;
                        if (paramName != null && (paramName.Equals("TypeDefinition", StringComparison.OrdinalIgnoreCase) || paramName.Equals("MemberDefinition", StringComparison.OrdinalIgnoreCase))
                            && i + 1 < elems.Length && elems[i + 1] is StringConstantExpressionAst nextStr)
                        {
                            executableSpans.Add(new[] { nextStr.Extent.StartOffset, nextStr.Extent.EndOffset });
                            break;
                        }
                    }
                }
            }
            if (node is InvokeMemberExpressionAst inv)
            {
                var memberName = (inv.Member as MemberExpressionAst)?.Extent?.Text ?? inv.Member?.Extent?.Text ?? "";
                var exprText = inv.Expression?.Extent?.Text ?? inv.Expression?.ToString() ?? "";
                if (memberName.Equals("Create", StringComparison.OrdinalIgnoreCase)
                    && exprText.IndexOf("ScriptBlock", StringComparison.OrdinalIgnoreCase) >= 0)
                {
                    foreach (var arg in inv.Arguments)
                    {
                        if (arg is StringConstantExpressionAst argStr)
                        {
                            executableSpans.Add(new[] { argStr.Extent.StartOffset, argStr.Extent.EndOffset });
                            break;
                        }
                    }
                }
            }
        }

        foreach (var node in ast.FindAll(n => n is CommandAst, searchNestedScriptBlocks: true))
        {
            if (node is CommandAst cmd && cmd.GetCommandName()?.Equals("Export-ModuleMember", StringComparison.OrdinalIgnoreCase) == true)
            {
                var elems = cmd.CommandElements.ToArray();
                for (int i = 0; i < elems.Length; i++)
                {
                    if (elems[i] is CommandParameterAst cp && cp.ParameterName?.Equals("Function", StringComparison.OrdinalIgnoreCase) == true
                        && i + 1 < elems.Length)
                    {
                        var next = elems[i + 1];
                        var val = next switch
                        {
                            StringConstantExpressionAst s => s.Value,
                            _ => next?.Extent?.Text
                        };
                        if (val != null)
                        {
                            foreach (var part in val.Split(',', ';', ' ').Select(p => p.Trim()).Where(p => p.Length > 0))
                                exportedFunctions.Add(part);
                        }
                        break;
                    }
                }
            }
        }

        return new Result
        {
            ExecutableStrings = executableSpans,
            ExportedFunctions = exportedFunctions.ToList()
        };
    }
    catch (Exception ex)
    {
        return new Result { Error = Escape(ex.Message) };
    }
}

static void Emit(Result r)
{
    var sb = new StringBuilder();
    sb.Append("{\"executableStrings\":[");
    if (r.ExecutableStrings != null)
    {
        for (int i = 0; i < r.ExecutableStrings.Count; i++)
        {
            if (i > 0) sb.Append(',');
            var s = r.ExecutableStrings[i];
            sb.Append($"[{s[0]},{s[1]}]");
        }
    }
    sb.Append("],\"exportedFunctions\":[");
    if (r.ExportedFunctions != null)
    {
        for (int i = 0; i < r.ExportedFunctions.Count; i++)
        {
            if (i > 0) sb.Append(',');
            sb.Append('"').Append(Escape(r.ExportedFunctions[i])).Append('"');
        }
    }
    sb.Append(']');
    if (r.Error != null)
        sb.Append(",\"error\":\"").Append(r.Error).Append('"');
    sb.Append('}');
    Console.OutputEncoding = Encoding.UTF8;
    Console.Write(sb.ToString());
}

static string Escape(string s)
{
    if (string.IsNullOrEmpty(s)) return s ?? "";
    return s.Replace("\\", "\\\\").Replace("\"", "\\\"").Replace("\n", "\\n").Replace("\r", "\\r").Replace("\t", "\\t");
}

record Result
{
    public List<int[]>? ExecutableStrings { get; init; }
    public List<string>? ExportedFunctions { get; init; }
    public string? Error { get; init; }
}
