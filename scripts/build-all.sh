#!/bin/bash
set -e

echo "Running standard multi-platform build..."
bash "$(dirname "$0")/build.sh"

echo "Packaging release artifacts..."
bash "$(dirname "$0")/package.sh"

echo "Done. Supported targets: Windows x64, Windows Arm64, macOS Apple Silicon, macOS Intel, Linux x64."
