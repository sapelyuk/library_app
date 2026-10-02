# verify-db.ps1 — confirm the Book RAG database is up and the schema is correct
# Usage: .\scripts\verify-db.ps1
# Read-only: it creates nothing and changes nothing.

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    Write-Host "== Book RAG: verifying database ==" -ForegroundColor Cyan
    $failures = 0

    # --- container running? -------------------------------------------------
    $state = docker inspect --format '{{.State.Status}}' n8n-book-rag-db 2>$null
    if ($state -ne 'running') {
        Write-Host "FAIL: container 'n8n-book-rag-db' is not running (state: $state)." -ForegroundColor Red
        Write-Host "      Run .\scripts\start-db.ps1 first." -ForegroundColor Yellow
        exit 1
    }
    Write-Host "OK: container is running" -ForegroundColor Green

    # --- resolve user/db from .env -----------------------------------------
    $user = 'bookrag'; $db = 'bookrag'
    if (Test-Path '.env') {
        $m = Select-String -Path '.env' -Pattern '^POSTGRES_USER=(.+)$'
        if ($m) { $user = $m.Matches.Groups[1].Value.Trim() }
        $m = Select-String -Path '.env' -Pattern '^POSTGRES_DB=(.+)$'
        if ($m) { $db = $m.Matches.Groups[1].Value.Trim() }
    }

    function Invoke-Sql {
        param([string]$Sql)
        docker exec n8n-book-rag-db psql -U $user -d $db -t -A -c $Sql 2>&1
    }

    # --- 1. connectivity ----------------------------------------------------
    $ping = Invoke-Sql 'select 1;'
    if ($ping -notmatch '^1') {
        Write-Host "FAIL: cannot query the database as '$user'." -ForegroundColor Red
        Write-Host "      $ping" -ForegroundColor DarkGray
        exit 1
    }
    Write-Host "OK: connected to database '$db' as '$user'" -ForegroundColor Green

    # --- 2. pgvector extension ---------------------------------------------
    $ext = (Invoke-Sql "select extversion from pg_extension where extname = 'vector';").Trim()
    if ($ext) { Write-Host "OK: pgvector extension version $ext" -ForegroundColor Green }
    else { Write-Host "FAIL: the 'vector' extension is not installed." -ForegroundColor Red; $failures++ }

    # --- 3. tables ----------------------------------------------------------
    foreach ($t in 'books', 'book_chunks') {
        $exists = (Invoke-Sql "select to_regclass('public.$t') is not null;").Trim()
        if ($exists -eq 't') { Write-Host "OK: table '$t' exists" -ForegroundColor Green }
        else { Write-Host "FAIL: table '$t' is missing." -ForegroundColor Red; $failures++ }
    }

    # --- 4. vector column width must match EMBEDDING_DIM --------------------
    # The column is halfvec(3072), not vector(3072): pgvector cannot HNSW-index a
    # plain vector above 2000 dimensions. atttypmod reports the dimension for both.
    $colType = (Invoke-Sql "select udt_name from information_schema.columns where table_name='book_chunks' and column_name='embedding';").Trim()
    $dim = (Invoke-Sql "select atttypmod from pg_attribute where attrelid = 'public.book_chunks'::regclass and attname = 'embedding';").Trim()
    $envDim = '3072'
    if (Test-Path '.env') {
        $m = Select-String -Path '.env' -Pattern '^EMBEDDING_DIM=(.+)$'
        if ($m) { $envDim = $m.Matches.Groups[1].Value.Trim() }
    }
    if ($dim -eq $envDim) {
        Write-Host "OK: embedding column is $colType($dim), matching EMBEDDING_DIM" -ForegroundColor Green
    }
    elseif ($dim) {
        Write-Host "WARN: embedding column is $colType($dim) but EMBEDDING_DIM=$envDim in .env." -ForegroundColor Yellow
        Write-Host "      The n8n embeddings node MUST emit exactly $dim dimensions or inserts will fail." -ForegroundColor Yellow
    }
    else {
        Write-Host "FAIL: could not read the embedding column type." -ForegroundColor Red
        $failures++
    }

    if ($colType -eq 'halfvec') {
        Write-Host "OK: halfvec is required for a 3072-dimension HNSW index (plain vector caps at 2000)" -ForegroundColor Green
    }
    elseif ($colType -eq 'vector') {
        Write-Host "WARN: column is plain 'vector'. Above 2000 dimensions pgvector cannot build an HNSW index," -ForegroundColor Yellow
        Write-Host "      so similarity search will fall back to a sequential scan. See docs/DECISIONS.md ADR-002." -ForegroundColor Yellow
    }

    # --- 4b. the HNSW index must exist, or every search is a full scan -------
    $hasIndex = (Invoke-Sql "select count(*) from pg_indexes where tablename = 'book_chunks' and indexname = 'book_chunks_embedding_idx';").Trim()
    if ([int]$hasIndex -ge 1) {
        Write-Host "OK: HNSW index 'book_chunks_embedding_idx' exists" -ForegroundColor Green
    }
    else {
        Write-Host "FAIL: HNSW index 'book_chunks_embedding_idx' is missing (searches will be sequential scans)." -ForegroundColor Red
        $failures++
    }

    # --- 5. functions -------------------------------------------------------
    foreach ($f in 'match_book_chunks', 'get_book', 'list_books', 'delete_book') {
        $exists = (Invoke-Sql "select count(*) from pg_proc where proname = '$f';").Trim()
        if ([int]$exists -ge 1) { Write-Host "OK: function '$f' exists" -ForegroundColor Green }
        else { Write-Host "FAIL: function '$f' is missing." -ForegroundColor Red; $failures++ }
    }

    # --- 6. row counts (informational) --------------------------------------
    $counts = (Invoke-Sql "select (select count(*) from books) || ' books, ' || (select count(*) from book_chunks) || ' chunks';").Trim()
    Write-Host "INFO: $counts" -ForegroundColor Cyan

    # --- verdict ------------------------------------------------------------
    Write-Host ""
    if ($failures -eq 0) {
        Write-Host "PASS: database and schema are ready for the n8n workflow." -ForegroundColor Green
        Write-Host "      If counts are 0, run the 'Index Book Library' trigger in n8n." -ForegroundColor Cyan
        exit 0
    }
    else {
        Write-Host "FAIL: $failures check(s) failed." -ForegroundColor Red
        Write-Host "      Apply the schema manually with:" -ForegroundColor Yellow
        Write-Host "      docker exec -i n8n-book-rag-db psql -U $user -d $db < db/01-schema.sql" -ForegroundColor Yellow
        exit 1
    }
}
finally {
    Pop-Location
}
