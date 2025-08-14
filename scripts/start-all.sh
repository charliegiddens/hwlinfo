#!/bin/bash

# Get the directory where this script is located (should be scripts/)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Get the project root directory (parent dir of scripts/)
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Starting Mondae..."

# Create necessary directories
mkdir -p "$PROJECT_ROOT/bin" "$PROJECT_ROOT/logs" "$PROJECT_ROOT/data" "$PROJECT_ROOT/config"

# Check if binaries exist
if [ ! -f "$PROJECT_ROOT/bin/producer" ] || [ ! -f "$PROJECT_ROOT/bin/consumer" ]; then
    echo "Build Go binaries using scripts/build.sh before starting Mondae"
    exit 1
fi

# Check if prometheus.yml exists
if [ ! -f "$PROJECT_ROOT/config/prometheus.yml" ]; then
    echo "prometheus.yml not found at $PROJECT_ROOT/config/prometheus.yml"
    exit 1
fi

# Check if sensor targets json exists
if [ ! -f "$PROJECT_ROOT/data/sensor_targets.json" ]; then
    echo "sensor_targets.json not found at $PROJECT_ROOT/data/sensor_targets.json"
    exit 1
fi

echo ""
echo "Spinning up Docker Compose..."
cd "$PROJECT_ROOT"
docker-compose up -d

if [ $? -ne 0 ]; then
    echo "Failed to start Docker services"
    exit 1
fi

# Wait for services to be ready
echo ""
echo "Waiting for services to be ready..."
sleep 15 # This is just an arbitrary amount of time but should be enough.
# TODO: ^ think about replacing this with an actual system that waits for each service 

echo ""
echo "Starting producer binary as a background process..."
cd "$PROJECT_ROOT"
nohup ./bin/producer > logs/producer.log 2>&1 &
echo $! > logs/producer.pid
echo "Producer binary started with PID $(cat logs/producer.pid)"

echo ""
echo "Starting consumer binary as a background process..."
nohup ./bin/consumer > logs/consumer.log 2>&1 &
echo $! > logs/consumer.pid
echo "Consumer binary started with PID $(cat logs/consumer.pid)"

echo ""
echo "Services:"
docker-compose ps
echo ""
echo "Exposed services:"
echo "Prometheus: http://localhost:9090"
echo "Grafana: http://localhost:3000 (admin/admin)"
echo "Kafka UI: http://localhost:8090"
echo ""