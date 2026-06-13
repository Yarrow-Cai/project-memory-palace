@echo off
setlocal enabledelayedexpansion
title Project Memory Palace Installer

:: ============================================================
::  Project Memory Palace - Quick Install Script
::  Installs pmem to %%USERPROFILE%%\.pmem and configures
::  Claude Desktop + Codex CLI MCP integrations.
:: ============================================================

set "PMEM_HOME=%USERPROFILE%\.pmem"
set "PMEM_BIN=%PMEM_HOME%\bin"
set "PMEM_EXE=%PMEM_BIN%\pmem.exe"
set "CLAUDE_CONFIG=%APPDATA%\Claude\claude_desktop_config.json"

echo.
echo  ╔══════════════════════════════════════════╗
echo  ║   Project Memory Palace  Installer       ║
echo  ╚══════════════════════════════════════════╝
echo.

:: ============================================================
::  Step 1: Find or build pmem.exe
:: ============================================================
echo  [1/4] Locating pmem.exe...

set "FOUND="

:: Check common locations
if exist "%PMEM_EXE%" (
    echo    Found existing: %PMEM_EXE%
    set "FOUND=1"
    set "SRC_EXE=%PMEM_EXE%"
) else if exist "pmem.exe" (
    echo    Found in current directory
    set "FOUND=1"
    set "SRC_EXE=%CD%\pmem.exe"
) else if exist "bin\pmem.exe" (
    echo    Found in bin\
    set "FOUND=1"
    set "SRC_EXE=%CD%\bin\pmem.exe"
) else (
    :: Try to build from Go source
    echo    Looking for Go source...
    if exist "go.mod" (
        echo    Building from source...
        go build -o bin\pmem.exe .\cmd\pmem 2>nul
        if exist "bin\pmem.exe" (
            echo    Build successful
            set "FOUND=1"
            set "SRC_EXE=%CD%\bin\pmem.exe"
        ) else (
            echo    ERROR: Build failed. Make sure Go 1.21+ is installed.
        )
    ) else (
        echo    ERROR: pmem.exe not found and no Go source detected.
        echo    Run this script from the project-memory-palace repository root,
        echo    or copy pmem.exe to the current directory first.
    )
)

if not defined FOUND (
    echo.
    echo  Installation aborted.
    pause
    exit /b 1
)

:: ============================================================
::  Step 2: Install binary
:: ============================================================
echo.
echo  [2/4] Installing to %PMEM_BIN%...

if not exist "%PMEM_BIN%" mkdir "%PMEM_BIN%"
copy /Y "%SRC_EXE%" "%PMEM_EXE%" >nul
if %errorlevel% neq 0 (
    echo    ERROR: Failed to copy binary
    pause
    exit /b 1
)
echo    Installed: %PMEM_EXE%

:: Add to user PATH if not already there
echo %%PATH%% | findstr /i /c:"%PMEM_BIN%" >nul
if %errorlevel% neq 0 (
    echo    Adding to PATH...
    setx PATH "%PATH%;%PMEM_BIN%" >nul
    echo    (restart terminal for PATH to take effect)
)

:: ============================================================
::  Step 3: Configure Claude Desktop MCP
:: ============================================================
echo.
echo  [3/4] Configuring Claude Desktop...

:: Claude MCP uses stdio mode, not SSE. pmem serve-mcp uses stdin/stdout.
set "MCP_ENTRY={\"command\":\"%PMEM_EXE:\=\\%\",\"args\":[\"serve-mcp\",\".\"]}"

:: Use PowerShell to merge JSON (handles existing config properly)
powershell -NoProfile -Command ^
    "$configPath = '%CLAUDE_CONFIG%';" ^
    "$mcpEntry = '%MCP_ENTRY%' | ConvertFrom-Json;" ^
    "if (Test-Path $configPath) {" ^
    "    $config = Get-Content $configPath -Raw | ConvertFrom-Json;" ^
    "} else {" ^
    "    $config = [PSCustomObject]@{mcpServers = [PSCustomObject]@{}};" ^
    "    New-Item -Path (Split-Path $configPath) -ItemType Directory -Force | Out-Null;" ^
    "}" ^
    "if (-not $config.mcpServers) { $config | Add-Member -NotePropertyName mcpServers -NotePropertyValue ([PSCustomObject]@{}) -Force };" ^
    "$config.mcpServers | Add-Member -NotePropertyName 'project-memory-palace' -NotePropertyValue $mcpEntry -Force;" ^
    "$config | ConvertTo-Json -Depth 4 | Set-Content $configPath -Encoding UTF8;" ^
    "Write-Host '    Configured:' $configPath"

if %errorlevel% equ 0 (
    echo    Claude Desktop configured ✓
) else (
    echo    WARNING: Claude Desktop config failed. You can manually add this to %CLAUDE_CONFIG%:
    echo.
    echo    {
    echo      "mcpServers": {
    echo        "project-memory-palace": {
    echo          "command": "%PMEM_EXE%",
    echo          "args": ["serve-mcp", "."]
    echo        }
    echo      }
    echo    }
)

:: ============================================================
::  Step 4: Configure Codex CLI MCP
:: ============================================================
echo.
echo  [4/4] Configuring Codex CLI...

where codex >nul 2>&1
if %errorlevel% neq 0 (
    echo    Codex CLI not found in PATH - skipping
    echo    Install manually with:
    echo      codex mcp add project-memory-palace -- "%PMEM_EXE%" serve-mcp .
) else (
    :: Remove existing if present, then add (idempotent)
    codex mcp remove project-memory-palace 2>nul
    codex mcp add project-memory-palace -- "%PMEM_EXE%" serve-mcp . 2>&1
    if %errorlevel% equ 0 (
        echo    Codex CLI configured ✓
    ) else (
        echo    WARNING: Codex config may have failed. Check with: codex mcp list
    )
)

:: ============================================================
::  Done
:: ============================================================
echo.
echo  ╔══════════════════════════════════════════╗
echo  ║   Installation Complete!                  ║
echo  ╚══════════════════════════════════════════╝
echo.
echo    pmem binary:  %PMEM_EXE%
echo    Config dir:   %PMEM_HOME%
echo.
echo  Quick Start:
echo    pmem init                    # Initialize in current directory
echo    pmem remember --file card.yaml  # Write a memory
echo    pmem search "keyword"        # Search memories
echo    pmem serve-web               # Launch Web UI at http://127.0.0.1:8147
echo.
echo  MCP servers will auto-start with Claude Desktop / Codex CLI.
echo.
endlocal
pause
