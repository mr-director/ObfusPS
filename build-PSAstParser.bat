@echo off
REM Build PSAstParser.exe (C# AST fallback for Windows when pwsh unavailable)
REM Requires .NET SDK: https://aka.ms/dotnet/download

set SCRIPT_DIR=%~dp0
set OUT_DIR=%SCRIPT_DIR%build\PSAstParser
mkdir "%OUT_DIR%" 2>nul

cd /d "%SCRIPT_DIR%scripts\PSAstParser"
where dotnet >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo dotnet not found. Install .NET SDK from https://aka.ms/dotnet/download
    pause
    exit /b 1
)

dotnet restore "%~dp0scripts\PSAstParser\PSAstParser.csproj"
dotnet publish "%~dp0scripts\PSAstParser\PSAstParser.csproj" -c Release -r win-x64 --self-contained false -o "%OUT_DIR%"

if %ERRORLEVEL% NEQ 0 (
    echo Build failed. Check errors above.
    pause
    exit /b 1
)

echo Built: %OUT_DIR%\PSAstParser.exe
pause
exit /b 0
