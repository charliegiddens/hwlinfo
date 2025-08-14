#!/bin/bash

echo "Stopping Mondae..."
echo ""

# Stop producer
echo "Stopping producer background process..."
if [ -f "logs/producer.pid" ]; then
    PID=$(cat logs/producer.pid)
    if kill -0 $PID 2>/dev/null; then
        kill $PID
        echo "Producer service stopped (PID: $PID)"
    else
        echo "Producer service was not running"
    fi
    rm logs/producer.pid
else
    echo "Producer PID file not found (may not have been running)"
fi

# Stop consumer
echo ""
echo "Stopping consumer background process..."
if [ -f "logs/consumer.pid" ]; then
    PID=$(cat logs/consumer.pid)
    if kill -0 $PID 2>/dev/null; then
        kill $PID
        echo "Consumer service stopped (PID: $PID)"
    else
        echo "Consumer service was not running"
    fi
    rm logs/consumer.pid
else
    echo "Consumer PID file not found (may not have been running)"
fi

# Stop Docker services
echo ""
echo "Stopping Docker services..."
docker-compose down

echo ""
echo "All services stopped"
echo ""