# import-workflow.ps1 — import (or update) the Book RAG workflow into n8n
# Usage: .\scripts\import-workflow.ps1
#
# Notes:
#  * Imports node structure and credential PLACEHOLDERS only. n8n never imports
#    secrets, so credentials must be attached in the UI afterwards (SETUP.md step 6).
#  * If a workflow with the same ID already exists, n8n updates it in place.
#  * n8n must NOT need to be running for the CLI import to work; the CLI writes
#    directly to the instance database.

[CmdletBinding()]
param(
    # Overwrite an existing workflow with the same ID instead of failing.
    [switch]$Update
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$workflowFile = Join-Path $projectRoot 'workflow\book-rag-system.json'
$n8nCmd = Join-Path $env:APPDATA 'npm\n8n.cmd'

Push-Location $projectRoot
try {
    Write-Host "== Book RAG: importing workflow ==" -ForegroundColor Cyan

    if (-not (Test-Path $workflowFile)) {
        Write-Host "FAIL: $workflowFile not found." -ForegroundColor Red
        exit 1
    }
    if (-not (Test-Path $n8nCmd)) {
        Write-Host "FAIL: n8n launcher not found at $n8nCmd" -ForegroundColor Red
        Write-Host "      Install with: npm install -g n8n" -ForegroundColor Yellow
        exit 1
    }

    # Validate the JSON before handing it to n8n, so a syntax error is obvious.
    try {
        $null = Get-Content $workflowFile -Raw | ConvertFrom-Json
        Write-Host "OK: workflow JSON parses" -ForegroundColor Green
    }
    catch {
        Write-Host "FAIL: workflow JSON is not valid JSON: $($_.Exception.Message)" -ForegroundColor Red
        exit 1
    }

    if ($Update) {
        Write-Host "Importing with --update..." -ForegroundColor Cyan
        & $n8nCmd import:workflow --input=$workflowFile --update
    }
    else {
        Write-Host "Importing..." -ForegroundColor Cyan
        & $n8nCmd import:workflow --input=$workflowFile
    }

    if ($LASTEXITCODE -ne 0) {
        Write-Host ""
        Write-Host "FAIL: import returned exit code $LASTEXITCODE." -ForegroundColor Red
        Write-Host "      If it complained the workflow already exists, re-run with -Update." -ForegroundColor Yellow
        exit $LASTEXITCODE
    }

    Write-Host ""
    Write-Host "OK: workflow imported." -ForegroundColor Green
    Write-Host "Next steps (docs\SETUP.md):" -ForegroundColor Cyan
    Write-Host "  1. Start n8n:  & `"$n8nCmd`" start"
    Write-Host "  2. Open http://localhost:5678 and create the four credentials (step 5)."
    Write-Host "  3. Attach them to the nodes listed in step 6."
    Write-Host "  4. Run the 'Index Book Library' trigger, then smoke-test (step 8)."
}
finally {
    Pop-Location
}
