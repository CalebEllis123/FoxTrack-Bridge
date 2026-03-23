#!/bin/bash
echo "Building FoxTrack Bridge for different platforms..."

# Build for Linux (default)
echo "Building for Linux..."
go build -o foxtrack-bridge-linux

# Build for Windows (if cross-compiling)
echo "Building for Windows..."
GOOS=windows GOARCH=amd64 go build -o foxtrack-bridge.exe

# Build for macOS
echo "Building for macOS..."
GOOS=darwin GOARCH=amd64 go build -o foxtrack-bridge-mac

echo "Build complete! Files created:"
ls -la foxtrack-bridge*
