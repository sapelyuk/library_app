# Regenerates Go gRPC code from the proto contracts of every module.
# Requires protoc unpacked into tools/protoc and the Go plugins installed:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$protoc = Join-Path $root "tools/protoc/bin/protoc.exe"
if (-not (Test-Path $protoc)) {
    throw "protoc not found at $protoc, unpack a protoc release into tools/protoc"
}

$env:PATH = "$(go env GOPATH)\bin;$env:PATH"

foreach ($module in @("book-service")) {
    $outDir = Join-Path $root "$module/gen/go"
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null

    $protoFiles = Get-ChildItem -Recurse -Filter *.proto (Join-Path $root "$module/proto") |
        ForEach-Object { $_.FullName }

    & $protoc "-I" "$module/proto" `
        "--go_out" "$module/gen/go" "--go_opt" "paths=source_relative" `
        "--go-grpc_out" "$module/gen/go" "--go-grpc_opt" "paths=source_relative" `
        $protoFiles

    if ($LASTEXITCODE -ne 0) {
        throw "protoc failed for $module"
    }

    Write-Host "generated gRPC code for $module"
}
