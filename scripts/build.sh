#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w -H windowsgui' -o dist/ThreeCatCompanion.exe .
cp cats.example.json dist/cats.json
printf '%s\n' 'Built dist/ThreeCatCompanion.exe. Private demo artwork must be supplied before a usable demo build.'
