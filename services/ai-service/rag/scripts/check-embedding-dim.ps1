# check-embedding-dim.ps1 — verify the exact embedding width your Gemini key returns
#
# WHY THIS EXISTS: n8n's "Embeddings Google Gemini" node exposes no dimension
# setting, and the Postgres column is a FIXED width (vector(3072)). If the model
# ever returns a different number of dimensions, every insert fails with a
# dimension-mismatch error. This script asks the API directly, so you find out
# in five seconds instead of mid-ingest.
#
# Usage:
#   .\scripts\check-embedding-dim.ps1
#   .\scripts\check-embedding-dim.ps1 -ApiKey "AIza..."
#
# It also lists the embedding models your key can actually see, so a retired or
# renamed model is obvious.

[CmdletBinding()]
param(
    [string]$ApiKey,
    [string]$Model = 'models/gemini-embedding-001'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    Write-Host "== Book RAG: checking Gemini embedding dimension ==" -ForegroundColor Cyan

    # Resolve the key: parameter wins, then .env, then GEMINI_API_KEY env var.
    if (-not $ApiKey) { $ApiKey = $env:GEMINI_API_KEY }
    if (-not $ApiKey -and (Test-Path '.env')) {
        $m = Select-String -Path '.env' -Pattern '^\s*GEMINI_API_KEY\s*=\s*(.+)$'
        if ($m) { $ApiKey = $m.Matches.Groups[1].Value.Trim() }
    }
    if (-not $ApiKey) {
        Write-Host "FAIL: no API key found." -ForegroundColor Red
        Write-Host "      Add GEMINI_API_KEY=... to .env, or pass -ApiKey, or set `$env:GEMINI_API_KEY." -ForegroundColor Yellow
        Write-Host "      (This .env entry is only used by this script; n8n stores the key in its credential.)" -ForegroundColor DarkGray
        exit 1
    }
    Write-Host "OK: API key found (length $($ApiKey.Length))" -ForegroundColor Green

    $headers = @{ 'x-goog-api-key' = $ApiKey }

    # --- 1. which embedding models does this key see? -----------------------
    Write-Host ""
    Write-Host "Embedding models visible to this key:" -ForegroundColor Cyan
    try {
        $models = Invoke-RestMethod -Method Get -Uri 'https://generativelanguage.googleapis.com/v1beta/models' -Headers $headers
        $embeddingModels = @($models.models | Where-Object { $_.name -match 'embedding' })
        if ($embeddingModels.Count -eq 0) {
            Write-Host "  WARN: none reported. The key may be restricted, or the list API changed." -ForegroundColor Yellow
        }
        foreach ($m in $embeddingModels) {
            Write-Host ("  - {0}" -f $m.name) -ForegroundColor Gray
        }
    }
    catch {
        Write-Host "  WARN: could not list models: $($_.Exception.Message)" -ForegroundColor Yellow
    }

    # --- 2. ask for one embedding and count the dimensions ------------------
    Write-Host ""
    Write-Host "Requesting one embedding from '$Model'..." -ForegroundColor Cyan
    $uri = "https://generativelanguage.googleapis.com/v1beta/$Model`:embedContent"
    $body = @{
        model   = $Model
        content = @{ parts = @(@{ text = 'dimension probe' }) }
    } | ConvertTo-Json -Depth 6

    try {
        $resp = Invoke-RestMethod -Method Post -Uri $uri -Headers $headers -ContentType 'application/json' -Body $body
    }
    catch {
        Write-Host "FAIL: embedContent call failed." -ForegroundColor Red
        Write-Host "      $($_.Exception.Message)" -ForegroundColor DarkGray
        Write-Host "      If this is a 404, the model name is wrong or retired - pick one from the list above." -ForegroundColor Yellow
        exit 1
    }

    $values = $resp.embedding.values
    $dim = @($values).Count
    if ($dim -eq 0) {
        Write-Host "FAIL: response contained no embedding values." -ForegroundColor Red
        Write-Host ($resp | ConvertTo-Json -Depth 5) -ForegroundColor DarkGray
        exit 1
    }

    # --- 3. compare against the schema --------------------------------------
    $envDim = '3072'
    if (Test-Path '.env') {
        $m = Select-String -Path '.env' -Pattern '^\s*EMBEDDING_DIM\s*=\s*(.+)$'
        if ($m) { $envDim = $m.Matches.Groups[1].Value.Trim() }
    }

    Write-Host ""
    Write-Host "Model '$Model' returns $dim dimensions." -ForegroundColor Cyan
    if ("$dim" -eq "$envDim") {
        Write-Host "MATCH: EMBEDDING_DIM=$envDim, so the database column vector($envDim) is correct." -ForegroundColor Green
        Write-Host "       No action needed." -ForegroundColor Green
        exit 0
    }

    Write-Host "MISMATCH: the API returns $dim but your schema expects $envDim." -ForegroundColor Red
    Write-Host ""
    Write-Host "Fix it in ONE of these two ways (they must agree):" -ForegroundColor Yellow
    Write-Host "  a) Change the column to match the API:" -ForegroundColor Yellow
    Write-Host "       ALTER TABLE book_chunks ALTER COLUMN embedding TYPE vector($dim);" -ForegroundColor Gray
    Write-Host "       then set EMBEDDING_DIM=$dim in .env, and re-index everything." -ForegroundColor Gray
    Write-Host "  b) Use a model that returns $envDim dimensions (see the model list above)." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Do NOT leave these mismatched: every ingest will fail." -ForegroundColor Red
    exit 1
}
finally {
    Pop-Location
}
