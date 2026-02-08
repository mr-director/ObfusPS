package engine

import "strings"

// reservedVars contains PowerShell automatic variables and reserved identifiers that must never be renamed.
// Renaming these would break script behavior (scope, closures, $MyInvocation, etc.).
// Reference: https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_automatic_variables
var reservedVars = map[string]bool{
	"$": true,
	// Core automatic variables
	"$args": true, "$input": true, "$null": true, "$true": true, "$false": true,
	"$error": true, "$foreach": true, "$?": true, "$^": true, "$_": true,
	"$host": true, "$pid": true, "$pwd": true, "$pshome": true, "$psversiontable": true,
	"$psboundparameters": true, "$myinvocation": true, "$pscmdlet": true,
	"$psscriptroot": true, "$pscommandpath": true,
	"$lastexitcode": true, "$ofs": true,
	"$stacktrace": true, "$sender": true, "$eventargs": true, "$event": true,
	"$nestedpromptlevel": true, "$matches": true, "$consolefilename": true,
	"$shellid": true, "$executioncontext": true,
	"$this": true, "$isglobal": true, "$isscript": true,
	// Scope modifiers — these precede ':varname' and must never be renamed.
	"$script": true, "$global": true, "$local": true, "$private": true,
	"$using": true, "$variable": true, "$workflow": true,
	// Additional automatic variables (PS 5.1+/7.x)
	"$psitem": true,             // Alias for $_
	"$psdebugcontext": true,     // Debugging context
	"$psculture": true,          // Current culture name
	"$psuiculture": true,        // UI culture name
	"$psedition": true,          // PowerShell edition (Core/Desktop)
	"$iswindows": true,          // OS detection (PS 7+)
	"$islinux": true,            // OS detection (PS 7+)
	"$ismacos": true,            // OS detection (PS 7+)
	"$iscoreclr": true,          // .NET Core detection
	"$profile": true,            // Profile path
	"$home": true,               // User home directory
	"$env": true,                // Environment variables
	"$switch": true,             // Switch statement enumerator
	"$psdefaultparametervalues": true, // Default parameter values
	"$outputencoding": true,     // Output encoding
	"$erroractionpreference": true,    // Error action preference
	"$warningpreference": true,  // Warning preference
	"$verbosepreference": true,  // Verbose preference
	"$debugpreference": true,    // Debug preference
	"$progresspreference": true, // Progress preference
	"$confirmpreference": true,  // Confirm preference
	"$whatifpreference": true,   // WhatIf preference
	"$informationpreference": true, // Information preference
}

func init() {
	// Normalize keys (lowercase for case-insensitive match)
	m := make(map[string]bool)
	for k := range reservedVars {
		m[strings.ToLower(k)] = true
	}
	reservedVars = m
}

// isReservedVariable returns true if the variable name (with or without $) must not be renamed.
// Handles scoped variables like $script:var, $global:var, $env:PATH, etc.
func isReservedVariable(name string) bool {
	if name == "" {
		return true
	}
	if !strings.HasPrefix(name, "$") {
		name = "$" + name
	}
	lower := strings.ToLower(name)
	// Direct match
	if reservedVars[lower] {
		return true
	}
	// Scoped variables: $scope:name — always preserve the scope prefix
	if strings.Contains(lower, ":") {
		parts := strings.SplitN(lower, ":", 2)
		scope := strings.TrimPrefix(parts[0], "$")
		// $env:* is always reserved (environment variables)
		if scope == "env" {
			return true
		}
		// Check if the base name (without scope) is reserved
		baseName := "$" + parts[1]
		if reservedVars[baseName] {
			return true
		}
	}
	return false
}
