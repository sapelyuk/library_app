# =============================================================================
# ingest-catalogue.ps1 - load a book catalogue through the ingest webhook
#
#   powershell -ExecutionPolicy Bypass -File .\scripts\ingest-catalogue.ps1
#   powershell -ExecutionPolicy Bypass -File .\scripts\ingest-catalogue.ps1 -Path .\samples\books.csv -DelayMs 500
#
# WHY THIS EXISTS
#   The workflow's `Index Book Library` path cannot read a file. `Read Book
#   Catalogue` is an extractFromFile node: it converts INCOMING BINARY data to
#   JSON, and nothing upstream of it produces any (the manual trigger emits
#   nothing). So the bulk path fails at that node.
#
#   This script drives the SAME indexing chain through `Ingest Webhook`
#   (`Prepare Book Record` -> chunk -> embed -> PGVector -> upsert), so the
#   library ends up identical. It is also idempotent: book_id is the key, and
#   re-running updates rather than duplicates.
#
# PREREQUISITES
#   - the database container is up            (.\scripts\start-db.ps1)
#   - n8n is running AND the workflow is Active (production /webhook/ URLs only
#     answer while the workflow is active)
#   - the three credentials from docs/SETUP.md step 5 are attached
#
# Requires the ingest webhook's header auth, read from .env. Changes nothing
# except the book library.
# =============================================================================

[CmdletBinding()]
param(
    [string] $Path     = 'samples\books.csv',
    [string] $BaseUrl  = 'http://localhost:5678',
    [int]    $DelayMs  = 500,
    [switch] $StopOnError
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root

function Section($t) { Write-Host "`n=== $t ===" -ForegroundColor Cyan }
function Ok($t)      { Write-Host "  OK   $t" -ForegroundColor Green }
function Bad($t)     { Write-Host "  FAIL $t" -ForegroundColor Red }
function Info($t)    { Write-Host "  ..   $t" -ForegroundColor Gray }

try {
    # --- config from .env (secrets never live in this file) -------------------
    if (-not (Test-Path '.env')) { throw ".env not found - run from the project folder." }
    $envText = Get-Content '.env' -Raw
    function EnvVal($name) {
        $m = [regex]::Match($envText, "(?m)^\s*$name\s*=\s*(.+)$")
        if ($m.Success) { $m.Groups[1].Value.Trim() } else { $null }
    }
    $headerName  = EnvVal 'WEBHOOK_HEADER_NAME'
    $headerValue = EnvVal 'WEBHOOK_HEADER_VALUE'
    if (-not $headerName -or -not $headerValue) { throw "WEBHOOK_HEADER_NAME / WEBHOOK_HEADER_VALUE missing from .env" }

    $uri = "$BaseUrl/webhook/book-rag/ingest"
    $headers = @{ $headerName = $headerValue }

    # --- preflight ------------------------------------------------------------
    Section "Preflight"
    try {
        $health = Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 10
        Ok "n8n is up ($($health.status))"
    } catch { throw "n8n is not answering on $BaseUrl - start it first." }

    if (-not (Test-Path $Path)) { throw "catalogue not found: $Path" }
    $rows = Import-Csv -Path $Path
    if (-not $rows) { throw "$Path has no data rows." }
    Ok "$Path -> $($rows.Count) row(s)"

    # --- ingest ---------------------------------------------------------------
    Section "Ingesting via POST $uri"
    $okCount = 0
    $failures = @()
    $i = 0
    foreach ($row in $rows) {
        $i++
        $payload = @{
            book_id        = $row.book_id
            title          = $row.title
            author         = $row.author
            genre          = $row.genre
            tags           = $row.tags
            published_year = $row.published_year
            page_count     = $row.page_count
            language       = $row.language
            rating         = $row.rating
            description    = $row.description
        }
        if ($row.PSObject.Properties.Name -contains 'content') { $payload.content = $row.content }

        $label = if ($row.title) { $row.title } else { $row.book_id }
        try {
            $body = $payload | ConvertTo-Json -Depth 5
            $resp = Invoke-RestMethod -Method Post -Uri $uri -Headers $headers -ContentType 'application/json; charset=utf-8' -Body $body -TimeoutSec 180
            $okCount++
            Ok ("[{0,2}/{1}] {2}" -f $i, $rows.Count, $label)
        } catch {
            $status = ''
            if ($_.Exception.Response) { $status = "HTTP $([int]$_.Exception.Response.StatusCode)" }
            $failures += "$label ($status)"
            Bad ("[{0,2}/{1}] {2}  {3}" -f $i, $rows.Count, $label, $_.Exception.Message)
            if ($StopOnError) { throw "stopping on first failure (-StopOnError)" }
        }
        if ($DelayMs -gt 0 -and $i -lt $rows.Count) { Start-Sleep -Milliseconds $DelayMs }
    }

    # --- result ---------------------------------------------------------------
    Section "Result"
    Write-Host "  ingested : $okCount / $($rows.Count)" -ForegroundColor $(if ($failures.Count -eq 0) { 'Green' } else { 'Yellow' })
    if ($failures.Count) {
        Write-Host "  failures :" -ForegroundColor Red
        $failures | ForEach-Object { Write-Host "    - $_" -ForegroundColor Red }
        Info "A 500 usually means the Gemini embedding quota; a 403 means the header value does not match .env."
    }

    Info "Verify with: docker exec n8n-book-rag-db psql -U bookrag -d bookrag -c `"select (select count(*) from books) as books, (select count(*) from book_chunks) as chunks;`""
    Info "Re-running is idempotent - counts must not grow."

    if ($failures.Count) { exit 1 } else { exit 0 }
}
finally {
    Pop-Location
}
