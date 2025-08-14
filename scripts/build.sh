#!/bin/bash

echo "Building Go services -> binaries..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build producer service binary
echo "Building producer..."
go build -o bin/producer services/producer/main.go
if [ $? -eq 0 ]; then
    echo "Producer binary built successfully"
else
    echo "Failed to build producer binary"
    exit 1
fi

# Build consumer service binary
echo "Building consumer..."
go build -o bin/consumer services/consumer/main.go
if [ $? -eq 0 ]; then
    echo "Consumer binary built successfully"
else
    echo "Failed to build consumer binary"
    exit 1
fi

echo "Consumer/Producer binaries built successfully"
echo ""