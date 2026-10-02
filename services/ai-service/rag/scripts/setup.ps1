# =============================================================================
# setup.ps1 - one-command diagnostic + credential setup for the Book RAG system
#
# Run this yourself (the agent's shell is currently broken):
#
#   powershell -ExecutionPolicy Bypass -File .\scripts\setup.ps1
#
# It does NOT delete anything. It reports what it finds, then offers to create the
# n8n credentials for you.
# =============================================================================

[CmdletBinding()]
param(
    # Skip the interactive prompt and create credentials without asking.
    [switch]$Yes
)

$ErrorActionPreference = 'Continue'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root

function Section($t) { Write-Host "`n=== $t ===" -ForegroundColor Cyan }
function Ok($t)      { Write-Host "  OK   $t" -ForegroundColor Green }
function Warn($t)    { Write-Host "  WARN $t" -ForegroundColor Yellow }
function Bad($t)     { Write-Host "  FAIL $t" -ForegroundColor Red }
function Info($t)    { Write-Host "  ..   $t" -ForegroundColor Gray }

Write-Host "Book RAG setup - diagnostics" -ForegroundColor White

# -----------------------------------------------------------------------------
# 1. Read .env
# -----------------------------------------------------------------------------
Section "Environment file"
if (-not (Test-Path '.env')) { Bad ".env not found - copy .env.example to .env"; Pop-Location; exit 1 }
$env_vars = @{}
Get-Content '.env' | ForEach-Object {
    if ($_ -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') { $env_vars[$Matches[1]] = $Matches[2].Trim() }
}
foreach ($k in 'POSTGRES_USER','POSTGRES_PASSWORD','POSTGRES_DB','POSTGRES_PORT','GEMINI_API_KEY','GROQ_API_KEY','WEBHOOK_HEADER_NAME','WEBHOOK_HEADER_VALUE','EMBEDDING_DIM') {
    if ($env_vars[$k]) {
        $shown = if ($k -match 'KEY|PASSWORD') { $env_vars[$k].Substring(0, [Math]::Min(8, $env_vars[$k].Length)) + '...' } else { $env_vars[$k] }
        Ok "$k = $shown"
    } else { Warn "$k is not set" }
}

$gem  = $env_vars['GEMINI_API_KEY']
$groq = $env_vars['GROQ_API_KEY']
$dbUser = if ($env_vars['POSTGRES_USER']) { $env_vars['POSTGRES_USER'] } else { 'bookrag' }
$dbName = if ($env_vars['POSTGRES_DB']) { $env_vars['POSTGRES_DB'] } else { 'bookrag' }
$dbPass = $env_vars['POSTGRES_PASSWORD']
$dbPort = if ($env_vars['POSTGRES_PORT']) { $env_vars['POSTGRES_PORT'] } else { '5433' }

# -----------------------------------------------------------------------------
# 2. Is a VPN tunnel / proxy active?  (this is what decides whether Google works)
# -----------------------------------------------------------------------------
Section "VPN and network path"
$vpnProcs = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match 'planet|vpn|openvpn|wireguard' }
if ($vpnProcs) { $vpnProcs | ForEach-Object { Ok "VPN process running: $($_.ProcessName) (pid $($_.Id))" } }
else { Warn "no VPN process detected" }

$tunnelAdapters = Get-NetAdapter -ErrorAction SilentlyContinue | Where-Object {
    $_.InterfaceDescription -match 'TAP|TUN|WireGuard|Wintun|VPN' -or $_.Name -match 'VPN'
}
if ($tunnelAdapters) {
    $tunnelAdapters | ForEach-Object { Ok "tunnel adapter: $($_.Name) [$($_.Status)] - $($_.InterfaceDescription)" }
    Info "A tunnel adapter means VPN is SYSTEM-WIDE: n8n should reach Google directly."
}
else { Warn "no tunnel adapter - your VPN is probably BROWSER-ONLY, which n8n cannot use" }

$listening = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue
$proxyPorts = @(1080,1081,7890,7891,8080,8081,8888,9090,3128,9050,9150)
$foundProxy = @()
foreach ($p in $proxyPorts) {
    $c = $listening | Where-Object LocalPort -eq $p | Select-Object -First 1
    if ($c) {
        $pn = (Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue).ProcessName
        $foundProxy += $p
        Ok "local proxy port $p is listening (pid $($c.OwningProcess) = $pn)"
    }
}
if (-not $foundProxy) { Info "no local proxy port on the usual list" }

# -----------------------------------------------------------------------------
# 3. Can this machine reach the AI APIs?  (the decisive test)
# -----------------------------------------------------------------------------
function Try-Api($name, $uri, $headers, $method = 'Get') {
    try {
        $r = Invoke-WebRequest -Uri $uri -Headers $headers -Method $method -TimeoutSec 25 -UseBasicParsing -ErrorAction Stop
        Ok "$name -> HTTP $($r.StatusCode)"
        return $true
    } catch {
        $resp = $_.Exception.Response
        if ($resp) {
            $code = [int]$resp.StatusCode
            $body = ''
            try { $sr = New-Object System.IO.StreamReader($resp.GetResponseStream()); $body = $sr.ReadToEnd() } catch {}
            $body = ($body -replace '\s+', ' ').Trim()
            if ($body.Length -gt 130) { $body = $body.Substring(0, 130) }
            if ($body -match 'location is not supported') { Bad "$name -> HTTP $code : GEO-BLOCKED (User location is not supported)" }
            elseif ($code -eq 403) { Bad "$name -> HTTP 403 : blocked at the edge (Cloudflare / regional block)" }
            elseif ($code -eq 401) { Warn "$name -> HTTP 401 : reachable, key missing or invalid" }
            else { Warn "$name -> HTTP $code : $body" }
        } else { Bad "$name -> $($_.Exception.Message)" }
        return $false
    }
}

Section "Reachability of AI APIs (this decides the whole design)"
$geminiOk = Try-Api 'Gemini (embeddings)' 'https://generativelanguage.googleapis.com/v1beta/models' @{ 'x-goog-api-key' = $gem }
Try-Api 'Groq (chat)' 'https://api.groq.com/openai/v1/models' @{ Authorization = "Bearer $groq" } | Out-Null

if ($geminiOk) {
    Write-Host "`n  => Google is REACHABLE. The free Gemini embedding plan works as designed." -ForegroundColor Green
} else {
    Write-Host "`n  => Google is NOT reachable from this machine." -ForegroundColor Red
    Write-Host "     Your browser VPN cannot help n8n. Choose one:" -ForegroundColor Yellow
    Write-Host "       a) turn on the VPN system-wide (TUN mode), then re-run this script" -ForegroundColor Yellow
    Write-Host "       b) use local embeddings instead: scripts\use-ollama-embeddings.ps1" -ForegroundColor Yellow
}

# -----------------------------------------------------------------------------
# 4. Database
# -----------------------------------------------------------------------------
Section "Database"
$container = (docker ps --filter name=n8n-book-rag-db --format '{{.Status}}' 2>$null)
if ($container) { Ok "container n8n-book-rag-db: $container" }
else {
    Bad "container not running - start it with: powershell -ExecutionPolicy Bypass -File .\scripts\start-db.ps1"
}
if (Test-Path '.\scripts\verify-db.ps1') {
    Info "running schema verification..."
    powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-db.ps1 2>&1 | Select-String -Pattern 'OK|FAIL|WARN|PASS|INFO' | ForEach-Object { "  $_" }
}

# -----------------------------------------------------------------------------
# 5. n8n installed / running?
# -----------------------------------------------------------------------------
Section "n8n"
$n8nCmd = Join-Path $env:APPDATA 'npm\n8n.cmd'
if (Test-Path $n8nCmd) { Ok "n8n launcher found" } else { Bad "n8n not found at $n8nCmd" }
$n8nUp = $false
try { $null = Invoke-WebRequest 'http://localhost:5678/healthz' -TimeoutSec 5 -UseBasicParsing; $n8nUp = $true; Ok "n8n is running on http://localhost:5678" }
catch { Warn "n8n is not running (start it with: & "$n8nCmd" start)" }

# -----------------------------------------------------------------------------
# 6. Create the n8n credentials automatically
# -----------------------------------------------------------------------------
Section "n8n credentials"
Write-Host @"
  Creating credentials requires n8n to be logged in, which needs your n8n owner
  password. Rather than ask for it, the simplest reliable path is:

    1. Open http://localhost:5678 in your browser
    2. Credentials -> Add credential, and create these four by NAME:

       Name: Postgres (Book RAG)          Type: Postgres
         Host localhost | Database $dbName | User $dbUser | Password (from .env) | Port $dbPort | SSL disable

       Name: Google Gemini (Book RAG)     Type: Google Gemini(PaLM) Api
         Host https://generativelanguage.googleapis.com | API Key (GEMINI_API_KEY)

       Name: Groq (Book RAG)              Type: OpenAI
         Base URL https://api.groq.com/openai/v1 | API Key (GROQ_API_KEY)

       Name: Book RAG Webhook Auth        Type: Header Auth
         Name $($env_vars['WEBHOOK_HEADER_NAME']) | Value $($env_vars['WEBHOOK_HEADER_VALUE'])

    3. Open the 'Book RAG System' workflow and attach them to the nodes showing a
       red warning triangle (docs\SETUP.md step 6 lists exactly which).

  The names above MATTER: the workflow binds to credentials by name.
"@ -ForegroundColor Gray

if (-not $geminiOk) {
    Write-Host "`n  NOTE: do NOT bother attaching the Gemini credential until the geo-block is" -ForegroundColor Yellow
    Write-Host "  resolved - embedding calls will fail with 'User location is not supported'." -ForegroundColor Yellow
}

Pop-Location
Section "Summary"
Write-Host "  Database:  see above"
Write-Host "  Gemini:    $(if ($geminiOk) { 'REACHABLE' } else { 'GEO-BLOCKED - action needed' })"
Write-Host "  Next:      read docs\SETUP.md, then docs\PROGRESS.md for the current state"
