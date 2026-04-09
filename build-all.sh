#!/bin/bash
set -e

echo "Running standard multi-platform build..."
bash ./build.sh

echo "Packaging release artifacts..."
bash ./package.sh

echo "Done. Supported targets: Windows x64, Windows Arm64, macOS Apple Silicon, macOS Intel, Linux x64."
