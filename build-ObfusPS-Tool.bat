@echo off
setlocal
cd /d "%~dp0"

echo [*] Building ObfusPS-Tool.exe (with file picker)...
echo.

REM Check Python
where python >nul 2>&1
if errorlevel 1 (
    echo [x] Python not found. Install Python 3.
    pause
    exit /b 1
)

REM Install PyInstaller and dependencies
echo [~] Installing PyInstaller...
pip install pyinstaller colorama --quiet
if errorlevel 1 (
    echo [x] pip install failed.
    pause
    exit /b 1
)

REM Build obfusps.exe (Go engine)
echo [~] Building obfusps.exe...
go build -o obfusps.exe ./cmd/obfusps 2>nul
if errorlevel 1 (
    echo [!] obfusps.exe build failed - ensure Go is installed.
    echo     ObfusPS-Tool.exe will look for obfusps.exe in same folder.
) else (
    echo [+] obfusps.exe built.
)

REM Build ObfusPS-Tool.exe with PyInstaller (--console = window for input + file picker)
echo [~] Building ObfusPS-Tool.exe...
pyinstaller --onefile --name ObfusPS-Tool --console ObfusPS-Tool.py

if exist "dist\ObfusPS-Tool.exe" (
    echo.
    echo [+] Done: dist\ObfusPS-Tool.exe
    if exist obfusps.exe (
        copy /Y obfusps.exe dist\obfusps.exe >nul 2>&1
        echo [+] obfusps.exe copied to dist\
    )
    echo.
    echo Place ObfusPS-Tool.exe and obfusps.exe in the same folder to use.
) else (
    echo [x] Build failed.
)
pause
