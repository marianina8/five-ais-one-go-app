#!/bin/bash
# Quick test of the URL shortener service

echo "Building the service..."
go build -o shortener .

echo "Testing without ADMIN_TOKEN (should fail)..."
if ADMIN_TOKEN= ./shortener -addr:0 2>&1 | grep -q "ADMIN_TOKEN"; then
    echo "✓ Correctly fails without ADMIN_TOKEN"
else
    echo "✗ Should fail without ADMIN_TOKEN"
    exit 1
fi

echo "Testing command-line flags parsing..."
ADMIN_TOKEN=test ./shortener -help 2>&1 | grep -q "Usage of"
if [ $? -eq 0 ]; then
    echo "✓ Command-line flags work correctly"
else
    echo "✗ Command-line flags not working"
    exit 1
fi

echo "All basic checks passed!"