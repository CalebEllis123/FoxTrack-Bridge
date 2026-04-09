#!/usr/bin/env bash
set -e
APP="foxtrack-bridge"
OUT="./dist"
mkdir -p "$OUT"

echo "Downloading dependencies..."
go mod tidy

echo "Building release targets..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o "$OUT/${APP}-windows-amd64.exe" .
GOOS=windows GOARCH=arm64 go build -ldflags="-s -w -H=windowsgui" -o "$OUT/${APP}-windows-arm64.exe" .
GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o "$OUT/${APP}-mac-arm64" .
GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o "$OUT/${APP}-mac-intel" .
GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o "$OUT/${APP}-linux-amd64" .

echo ""
echo "Build complete. Binaries in $OUT/"
ls -lh "$OUT/"
