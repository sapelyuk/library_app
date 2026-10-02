# start-db.ps1 — bring up the pgvector container for the Book RAG system
# Usage: .\scripts\start-db.ps1
# Safe to re-run. Never deletes data.

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    Write-Host "== Book RAG: starting database ==" -ForegroundColor Cyan

    # 1. .env must exist
    if (-not (Test-Path '.env')) {
        Write-Host "FAIL: .env not found." -ForegroundColor Red
        Write-Host "      Copy-Item .env.example .env   then set POSTGRES_PASSWORD." -ForegroundColor Yellow
        exit 1
    }

    # 2. Docker daemon must be running
    docker info --format '{{.ServerVersion}}' 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL: Docker daemon is not reachable." -ForegroundColor Red
        Write-Host "      Start Docker Desktop, wait for it to finish starting, then retry." -ForegroundColor Yellow
        exit 1
    }
    Write-Host "OK: Docker daemon reachable" -ForegroundColor Green

    # 3. Start the stack
    docker compose up -d
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL: 'docker compose up -d' failed. Read the output above." -ForegroundColor Red
        exit 1
    }

    # 4. Wait for the healthcheck
    Write-Host "Waiting for the database to report healthy..." -ForegroundColor Cyan
    $healthy = $false
    for ($i = 1; $i -le 40; $i++) {
        $status = (docker inspect --format '{{.State.Health.Status}}' n8n-book-rag-db 2>$null)
        if ($status -eq 'healthy') { $healthy = $true; break }
        Start-Sleep -Seconds 2
        # NB: no '??' operator here - this must stay valid in Windows PowerShell 5.1.
        if (-not $status) { $status = 'unknown' }
        Write-Host ("  [{0,2}/40] status: {1}" -f $i, $status)
    }

    if (-not $healthy) {
        Write-Host "FAIL: container did not become healthy within 80s." -ForegroundColor Red
        Write-Host "      Inspect with: docker logs n8n-book-rag-db --tail 50" -ForegroundColor Yellow
        exit 1
    }

    Write-Host "OK: database healthy" -ForegroundColor Green
    $port = (Select-String -Path '.env' -Pattern '^POSTGRES_PORT=(.+)$').Matches.Groups[1].Value
    if (-not $port) { $port = '5433' }
    Write-Host ""
    Write-Host "Database ready on localhost:$port" -ForegroundColor Green
    Write-Host "Next: .\scripts\verify-db.ps1" -ForegroundColor Cyan
}
finally {
    Pop-Location
}
