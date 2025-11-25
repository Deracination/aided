#!/bin/bash
# Build script for AIDE Checker .NET version

set -e

echo "Building AIDE Checker (.NET version)..."

# Check if dotnet is installed
if ! command -v dotnet &> /dev/null; then
    echo "Error: .NET SDK is not installed"
    echo "Please install .NET 8.0 SDK from: https://dotnet.microsoft.com/download"
    exit 1
fi

# Restore dependencies
echo "Restoring dependencies..."
dotnet restore

# Build
echo "Building..."
dotnet build -c Release

echo ""
echo "Build successful!"
echo "Run with: dotnet run"
echo "Or use the executable: ./bin/Release/net8.0/checker"
echo ""
echo "To create a self-contained single file:"
echo "  dotnet publish -c Release -r linux-x64 --self-contained -p:PublishSingleFile=true"
