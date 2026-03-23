#!/bin/bash
echo "Starting FoxTrack Bridge UI Server..."
echo "Access the configuration at: http://localhost:8080"
echo "To stop, press Ctrl+C"

# Check if port 8080 is already in use
if lsof -i :8080 > /dev/null; then
    echo "Port 8080 is already in use. Killing process..."
    kill -9 $(lsof -t -i :8080)
fi

# Start the web server
go run server.go
