# =============================================================================
# vpn-check.ps1 - does the VPN actually cover non-browser traffic?
#
# Run this AFTER connecting Planet VPN:
#
#   powershell -ExecutionPolicy Bypass -File .\scripts\vpn-check.ps1
#
# It answers three questions:
#   1. Did a VPN tunnel adapter come up?           (tunnel present?)
#   2. What public IP / country does the world see? (did the exit change?)
#   3. Does Google now accept the Gemini key?       (the thing that matters)
#
# Read-only. Changes nothing.
# =============================================================================

[CmdletBinding()]
param()

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root

function Section($t) { Write-Host "`n=== $t ===" -ForegroundColor Cyan }
function Ok($t)      { Write-Host "  OK   $t" -ForegroundColor Green }
function Warn($t)    { Write-Host "  WARN $t" -ForegroundColor Yellow }
function Bad($t)     { Write-Host "  FAIL $t" -ForegroundColor Red }
function Info($t)    { Write-Host "  ..   $t" -ForegroundColor Gray }

# --- read the key from .env ---------------------------------------------------
$gem = $null
if (Test-Path '.env') {
    $m = Select-String -Path '.env' -Pattern '^\s*GEMINI_API_KEY\s*=\s*(.+)$'
    if ($m) { $gem = $m.Matches.Groups[1].Value.Trim() }
}
if (-not $gem) { Bad "GEMINI_API_KEY not found in .env"; Pop-Location; exit 1 }

# --- 1. tunnel adapter --------------------------------------------------------
Section "1. VPN tunnel adapter"
$adapters = Get-NetAdapter -ErrorAction SilentlyContinue | Where-Object {
    $_.InterfaceDescription -match 'TAP|TUN|WireGuard|Wintun|VPN|OpenVPN' -or $_.Name -match 'VPN|TAP|TUN'
}
if ($adapters) {
    $adapters | ForEach-Object {
        $colour = if ($_.Status -eq 'Up') { 'Green' } else { 'Yellow' }
        Write-Host ("  {0,-28} {1,-10} {2}" -f $_.Name, $_.Status, $_.InterfaceDescription) -ForegroundColor $colour
    }
    $up = $adapters | Where-Object Status -eq 'Up'
    if ($up) { Ok "a tunnel adapter is UP - VPN is very likely system-wide" }
    else     { Warn "adapters exist but none is Up - the VPN may be disconnected" }
} else {
    Bad "no tunnel adapter found"
    Info "This usually means the VPN is BROWSER-ONLY, which cannot help n8n or scripts."
    Info "Look for a 'system-wide' / 'global' / 'TUN mode' toggle in Planet VPN."
}

# --- 2. what does the world see? ---------------------------------------------
Section "2. Public IP and country (as seen from a script, not the browser)"
foreach ($svc in @(
    @{ n='ipinfo.io';   u='https://ipinfo.io/json' },
    @{ n='ipapi.co';    u='https://ipapi.co/json/' }
)) {
    try {
        $r = Invoke-RestMethod -Uri $svc.u -TimeoutSec 20 -ErrorAction Stop
        $ip = if ($r.ip) { $r.ip } else { $r.query }
        $cc = if ($r.country) { $r.country } else { $r.country_code }
        $city = if ($r.city) { $r.city } else { $r.city }
        Ok "$($svc.n): IP $ip  country $cc  ($city)"
        break
    } catch { Warn "$($svc.n) lookup failed: $($_.Exception.Message)" }
}
Info "If this matches your real country, the VPN is NOT carrying script traffic."

# --- 3. the decisive test -----------------------------------------------------
Section "3. Does Google accept the Gemini key now?"
$uri = 'https://generativelanguage.googleapis.com/v1beta/models'
try {
    $r = Invoke-WebRequest -Uri $uri -Headers @{ 'x-goog-api-key' = $gem } -TimeoutSec 30 -UseBasicParsing -ErrorAction Stop
    Ok "Google responded HTTP $($r.StatusCode) - the key works from here"
    $models = ($r.Content | ConvertFrom-Json).models | Where-Object { $_.name -match 'embedding' }
    $models | ForEach-Object { Info "embedding model available: $($_.name)" }
}
catch {
    $resp = $_.Exception.Response
    if ($resp) {
        $code = [int]$resp.StatusCode
        $body = ''
        try { $sr = New-Object System.IO.StreamReader($resp.GetResponseStream()); $body = $sr.ReadToEnd() } catch {}
        if ($body -match 'location is not supported') {
            Bad "STILL GEO-BLOCKED: 'User location is not supported for the API use.'"
        } else {
            Bad "HTTP $code : $(($body -replace '\s+',' ').Trim())"
        }
    } else { Bad $_.Exception.Message }
}

# --- 4. embedding dimension (only meaningful if step 3 passed) ---------------
Section "4. Embedding dimension (the number your schema must match)"
try {
    $payload = @{
        model   = 'models/gemini-embedding-001'
        content = @{ parts = @(@{ text = 'dimension probe' }) }
    } | ConvertTo-Json -Depth 6
    $r = Invoke-RestMethod -Uri 'https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent' `
        -Method Post -Headers @{ 'x-goog-api-key' = $gem } -ContentType 'application/json' `
        -Body $payload -TimeoutSec 30 -ErrorAction Stop
    $dim = @($r.embedding.values).Count
    Ok "gemini-embedding-001 returns $dim dimensions"
    if ($dim -eq 3072) { Ok "matches db/01-schema.sql halfvec(3072) - no schema change needed" }
    else {
        Warn "schema expects 3072 but the API returns $dim"
        Info "Fix: ALTER TABLE book_chunks ALTER COLUMN embedding TYPE halfvec($dim);"
        Info "then set EMBEDDING_DIM=$dim in .env and re-build the HNSW index."
    }
}
catch { Warn "could not measure (step 3 must pass first): $($_.Exception.Message)" }

Pop-Location
Write-Host "`nDone. If step 3 said GEO-BLOCKED, the VPN does not cover script traffic." -ForegroundColor Cyan
Write-Host "Tell the agent the result and pick a different embedding route." -ForegroundColor Cyan
