$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
New-Item -ItemType Directory -Force dist | Out-Null
$env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -buildvcs=false -trimpath -ldflags='-s -w -H windowsgui' -o dist/ThreeCatCompanion.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Copy-Item cats.example.json dist/cats.json
Write-Host 'Built dist/ThreeCatCompanion.exe. Add authorized private art before packaging a usable gift.'
