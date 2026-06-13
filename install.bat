@echo off
setlocal enabledelayedexpansion
title Project Memory Palace Installer

:: ============================================================
::  Project Memory Palace - Quick Install
::  Downloads latest pmem.exe from GitHub Release and
::  configures Claude Desktop, Claude Code, and Codex CLI.
:: ============================================================

set "PMEM_HOME=%USERPROFILE%\.pmem"
set "PMEM_BIN=%PMEM_HOME%\bin"
set "PMEM_EXE=%PMEM_BIN%\pmem.exe"
set "RELEASE_URL=https://github.com/Yarrow-Cai/project-memory-palace/releases/latest/download/pmem.exe"

echo.
echo  +------------------------------------------------+
echo  ^|   Project Memory Palace  Installer             ^|
echo  +------------------------------------------------+
echo.

:: ============================================================
::  Step 1: Create directories
:: ============================================================
echo  [1/5] Creating directories...
if not exist "%PMEM_BIN%" mkdir "%PMEM_BIN%"
echo    %PMEM_BIN%

:: ============================================================
::  Step 2: Download latest pmem.exe
:: ============================================================
echo.
echo  [2/5] Downloading pmem.exe...

:: Try PowerShell first (Windows 7+)
where powershell >nul 2>&1
if %errorlevel% equ 0 (
    powershell -NoProfile -Command ^
        "[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12;" ^
        "Invoke-WebRequest -Uri '%RELEASE_URL%' -OutFile '%PMEM_EXE%' -UseBasicParsing;" ^
        "Write-Host '    Downloaded:' (Get-Item '%PMEM_EXE%').Length 'bytes'"
    set "DOWNLOAD_OK=!errorlevel!"
) else (
    :: Fallback to curl
    curl -sL "%RELEASE_URL%" -o "%PMEM_EXE%" 2>&1
    set "DOWNLOAD_OK=!errorlevel!"
    echo    Downloaded: %PMEM_EXE%
)

if not "%DOWNLOAD_OK%"=="0" (
    echo    ERROR: Download failed. Check network or try manually:
    echo      curl -L "%RELEASE_URL%" -o "%PMEM_EXE%"
    pause
    exit /b 1
)

:: ============================================================
::  Step 3: Add to PATH
:: ============================================================
echo.
echo  [3/5] Configuring PATH...
echo %%PATH%% | findstr /i /c:"%PMEM_BIN%" >nul
if %errorlevel% neq 0 (
    setx PATH "%PATH%;%PMEM_BIN%" >nul
    echo    Added to user PATH (restart terminal to take effect)
) else (
    echo    Already in PATH
)

:: ============================================================
::  Step 4: Configure Claude Desktop MCP
:: ============================================================
echo.
echo  [4/5] Configuring MCP integrations...

set "CLAUDE_CONFIG=%APPDATA%\Claude\claude_desktop_config.json"
:: Escape backslashes for JSON
set "ESC_EXE=%PMEM_EXE:\=\\%"
set "MCP_JSON={\"command\":\"!ESC_EXE!\",\"args\":[\"serve-mcp\",\".\"]}"

:: Claude Desktop (via PowerShell JSON merge)
if exist "%CLAUDE_CONFIG%" (
    powershell -NoProfile -Command ^
        "$configPath = '%CLAUDE_CONFIG%';" ^
        "$entry = '%MCP_JSON%' | ConvertFrom-Json;" ^
        "if (Test-Path $configPath) {" ^
        "    $config = Get-Content $configPath -Raw | ConvertFrom-Json;" ^
        "} else {" ^
        "    $config = [PSCustomObject]@{mcpServers = [PSCustomObject]@{}};" ^
        "    New-Item -Path (Split-Path $configPath) -ItemType Directory -Force | Out-Null;" ^
        "}" ^
        "if (-not (Get-Member -InputObject $config -Name 'mcpServers' -MemberType Properties)) {" ^
        "    $config | Add-Member -NotePropertyName mcpServers -NotePropertyValue ([PSCustomObject]@{}) -Force;" ^
        "}" ^
        "$config.mcpServers | Add-Member -NotePropertyName 'project-memory-palace' -NotePropertyValue $entry -Force;" ^
        "$config | ConvertTo-Json -Depth 4 | Set-Content $configPath -Encoding UTF8;" ^
        "Write-Host '    Claude Desktop: configured'"
    echo.
) else (
    echo    Claude Desktop: not found (skip)
)

:: Claude Code (claude mcp add)
where claude >nul 2>&1
if %errorlevel% equ 0 (
    claude mcp remove project-memory-palace 2>nul
    claude mcp add project-memory-palace -- "%PMEM_EXE%" serve-mcp . 2>nul
    if !errorlevel! equ 0 (
        echo    Claude Code: configured
    ) else (
        echo    Claude Code: install manually: claude mcp add project-memory-palace -- "%PMEM_EXE%" serve-mcp .
    )
) else (
    echo    Claude Code: not installed (skip)
)

:: Codex CLI (codex mcp add)
where codex >nul 2>&1
if %errorlevel% equ 0 (
    codex mcp remove project-memory-palace 2>nul
    codex mcp add project-memory-palace -- "%PMEM_EXE%" serve-mcp . 2>nul
    if !errorlevel! equ 0 (
        echo    Codex CLI: configured
    ) else (
        echo    Codex CLI: install manually: codex mcp add project-memory-palace -- "%PMEM_EXE%" serve-mcp .
    )
) else (
    echo    Codex CLI: not installed (skip)
)

:: ============================================================
::  Step 5: Verify
:: ============================================================
echo.
echo  [5/5] Verifying...
"%PMEM_EXE%" 2>&1 | findstr /c:"usage:" >nul
if %errorlevel% equ 0 (
    echo    pmem.exe works!
) else (
    echo    WARNING: pmem.exe may not run correctly
)

:: ============================================================
::  Done
:: ============================================================
echo.
echo  +------------------------------------------------+
echo  ^|   Installation Complete!                       ^|
echo  +------------------------------------------------+
echo.
echo    Binary:  %PMEM_EXE%
echo.
echo  Quick Start:
echo    pmem init                    # Initialize project
echo    pmem remember --file card.yaml  # Write memory
echo    pmem search "keyword"        # Search
echo    pmem serve-web               # Web UI (http://127.0.0.1:8147)
echo.
echo  MCP servers auto-start with Claude / Codex.
echo  Verify: claude mcp list  or  codex mcp list
echo.
endlocal
