#!/bin/bash
echo "Testing API endpoints..."

echo "1. Getting configuration:"
curl -s http://localhost:8080/api/config

echo ""
echo "2. Getting printer list:"
curl -s http://localhost:8080/api/printers

echo ""
echo "3. Getting status (this should show real data after connecting to printers):"
curl -s http://localhost:8080/api/status

echo ""
echo "4. Testing with a simple GET request to root:"
curl -s http://localhost:8080/ | head -20
