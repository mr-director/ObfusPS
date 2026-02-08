@echo off
setlocal

REM ---- UTF-8 console (required for banner Unicode characters) ----
chcp 65001 >nul 2>&1

REM ---- Save script directory early (before any if-blocks) ----
REM This avoids CMD parsing issues when path contains () like "ObfusPS-main (1)"
set "SCRIPT_DIR=%~dp0"

cd /d "%SCRIPT_DIR%"
if errorlevel 1 goto :bad_dir

REM Style tags (no delayed expansion needed - avoids conflict with ! in tags)
set "TAG_INPUT=[>] "
set "TAG_INFO=[!] "
set "TAG_ERROR=[x] "
set "TAG_ADD=[+] "
set "TAG_WAIT=[~] "

title ObfusPS - Obfuscator Tool

echo %TAG_INFO% ObfusPS - Interface

REM ---- Check that ObfusPS-Tool.py exists ----
if exist "%SCRIPT_DIR%ObfusPS-Tool.py" goto :script_found
echo %TAG_ERROR% ObfusPS-Tool.py not found.
echo %TAG_INFO% Expected location: %SCRIPT_DIR%
pause
exit /b 1
:script_found

REM ---- Find Python ----
REM On Windows, try "python" first (standard), then "py" launcher, then "python3".
REM Each candidate is verified with --version to skip Windows Store stubs.
set "PYTHON_CMD="

where python >nul 2>&1
if errorlevel 1 goto :try_py
python --version >nul 2>&1
if errorlevel 1 goto :try_py
set "PYTHON_CMD=python"
goto :found_python

:try_py
where py >nul 2>&1
if errorlevel 1 goto :try_python3
py --version >nul 2>&1
if errorlevel 1 goto :try_python3
set "PYTHON_CMD=py"
goto :found_python

:try_python3
where python3 >nul 2>&1
if errorlevel 1 goto :no_python
python3 --version >nul 2>&1
if errorlevel 1 goto :no_python
set "PYTHON_CMD=python3"
goto :found_python

:no_python
echo %TAG_ERROR% Python not installed or not in PATH.
echo %TAG_INFO% Install Python 3 from https://python.org/downloads/ then run ObfusPS.bat again.
pause
exit /b 1

:found_python

REM ---- Verify Python version is 3.x ----
REM Note: cannot use >= or < in CMD (redirection operators), so use == to detect Python 2
%PYTHON_CMD% -c "import sys;exit(1 if sys.version_info[0]==2 else 0)" >nul 2>&1
if errorlevel 1 goto :bad_python_version

REM ---- Check for colorama module ----
%PYTHON_CMD% -c "import colorama" >nul 2>&1
if not errorlevel 1 goto :colorama_ok

echo %TAG_WAIT% Module colorama missing. Installing...
%PYTHON_CMD% -m pip install colorama
if errorlevel 1 goto :colorama_fail
echo %TAG_ADD% colorama installed.

:colorama_ok
echo %TAG_INFO% Launching ObfusPS-Tool...
%PYTHON_CMD% "%SCRIPT_DIR%ObfusPS-Tool.py"
pause
exit /b 0

REM ---- Error labels (outside normal flow to avoid () parsing issues) ----

:bad_dir
echo %TAG_ERROR% Cannot change to script directory.
pause
exit /b 1

:bad_python_version
echo %TAG_ERROR% Python 3 is required. Found Python 2.
echo %TAG_INFO% Install Python 3 from https://python.org/downloads/
pause
exit /b 1

:colorama_fail
echo %TAG_ERROR% Failed to install colorama. Run manually: pip install colorama
pause
exit /b 1
