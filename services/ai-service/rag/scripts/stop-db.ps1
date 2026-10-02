# stop-db.ps1 — stop the pgvector container (data is KEPT)
# Usage: .\scripts\stop-db.ps1
# This never destroys data. To also delete the volume you would have to run
# 'docker compose down -v' yourself, deliberately.

[CmdletBinding()]
param(
    # Pass -RemoveVolume to ALSO delete the data volume. Destructive and permanent.
    [switch]$RemoveVolume
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    if ($RemoveVolume) {
        Write-Host "WARNING: -RemoveVolume will PERMANENTLY DELETE all indexed books." -ForegroundColor Red
        $answer = Read-Host "Type DELETE to confirm"
        if ($answer -ne 'DELETE') {
            Write-Host "Aborted. Nothing was changed." -ForegroundColor Yellow
            exit 0
        }
        docker compose down -v
    }
    else {
        docker compose down
    }

    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL: docker compose down returned an error." -ForegroundColor Red
        exit 1
    }

    if ($RemoveVolume) {
        Write-Host "OK: container stopped and data volume deleted." -ForegroundColor Yellow
    }
    else {
        Write-Host "OK: container stopped. Indexed data is preserved in volume n8n_book_rag_pgdata." -ForegroundColor Green
    }
}
finally {
    Pop-Location
}
