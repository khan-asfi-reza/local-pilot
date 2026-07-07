# local-pilot entry point (PowerShell).
#   .\pilot.ps1 start | add <model> | code [--dir X] | run --dir X --task "..."
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
go build -o bin/pilot.exe ./cmd/pilot
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& "bin/pilot.exe" @args
