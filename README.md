# Prerequisites
- Git
- Docker
- Go 1.21+

# Architecture
1. Producer binary (reads hardware metrics from sources provided in `data/sensor_targets.json`, publishes messages to Kafka)
2. Kafka broker (handles message queueing/delivery between the Producer and Consumer - coordinated by Zookeeper)
3. Zookeeper (manages Kafka broker metadata/coordinates Kafka)
4. Consumer binary (subscribes to Kafka topics, processes incoming messages, and exposes metrics on `:8080` for Prometheus)
5. Prometheus (Scrapes metrics from the Consumer endpoint at `:8080`)
6. Grafana (uses Prometheus as a data source)
7. Kafka UI (self-explanatory, found at `{device_ip}:8090`)


Go binaries (producer, consumer) are compiled binaries, and are run as background processes. All other services (Kafka + UI, Zookeeper, Prometheus, and Grafana) are containerised using Docker Compose.


# Installation
1. Clone the repository
  ```
  git clone https://github.com/heychoogle/mondae.git
  ```
2. Navigate to the project root directory 
  ```
  cd mondae
  ```
3. Ensure all files in the `scripts/` directory are executable
  ```
  chmod +x scripts/*.sh
  ```
4. Create the .env file in the project root (pre-populated with default Kafka + Grafana variables)
  ```
  cp .env.example .env
  ```
5. Compile Go service binaries
  ```
  bash scripts/build.sh
  ```
6. Start the compiled binaries and Dockerised services with the combined start script
  ```
  bash scripts/start-all.sh
  ```
7. Access the Grafana dashboard at `{device_ip}:3000` using credentials set in `.env` (you may have to expose this port through your firewall, e.g. `ufw`)
8. You can stop the background processes and dockerised services using:
  ```
  bash scripts/stop-all.sh
  ```
