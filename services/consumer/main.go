package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/joho/godotenv"
)

type MetricMessage struct {
	Name      string            `json:"name"`
	Help      string            `json:"help"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
	Timestamp time.Time         `json:"timestamp"`
}

type SensorConfig struct {
	Pattern     string `json:"pattern"`
	Name        string `json:"name"`
	Help        string `json:"help"`
	OriginRegex string `json:"origin_regex,omitempty"`
}

type SensorFile struct {
	Sensors []SensorConfig `json:"sensors"`
}

var sensor_targets []SensorConfig
var kafkaProducer sarama.AsyncProducer

func loadSensorConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var config SensorFile
	err = json.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	sensor_targets = config.Sensors
	return nil
}

func initKafka() error {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = false
	config.Producer.Return.Errors = true
	config.Producer.RequiredAcks = sarama.WaitForLocal
	config.Producer.Retry.Max = 3

	// Get Kafka broker from environment or use default
	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")

	producer, err := sarama.NewAsyncProducer(brokers, config)
	if err != nil {
		return err
	}

	kafkaProducer = producer

	// Handle errors in a separate goroutine
	go func() {
		for err := range producer.Errors() {
			log.Printf("Failed to produce message: %v", err)
		}
	}()

	return nil
}

func sendMetricToKafka(metric MetricMessage) {
	topic := os.Getenv("KAFKA_TOPIC")

	jsonData, err := json.Marshal(metric)
	if err != nil {
		log.Printf("Failed to marshal metric: %v", err)
		return
	}

	message := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.StringEncoder(jsonData),
		Key:   sarama.StringEncoder(metric.Name + "_" + metric.Labels["origin"]),
	}

	kafkaProducer.Input() <- message
}

func main() {
	binaryPath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	binaryDir := filepath.Dir(binaryPath)
	envPath := filepath.Join(binaryDir, "..", ".env")

	if err := godotenv.Load(envPath); err != nil {
		log.Fatal("Error loading .env file:", err)
	}

	err = loadSensorConfig("data/sensor_targets.json")
	if err != nil {
		log.Fatal("Failed to load sensor config:", err)
	}

	err = initKafka()
	if err != nil {
		log.Fatal("Failed to initialize Kafka:", err)
	}
	defer kafkaProducer.Close()

	log.Printf("Starting monitoring with %d sensor types", len(sensor_targets))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i, sensorTarget := range sensor_targets {
		go pollSensorContinuously(ctx, sensorTarget, i+1)
	}

	select {}
}

func pollSensorContinuously(ctx context.Context, config SensorConfig, goroutineID int) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	log.Printf("Started goroutine %d for sensor: %s", goroutineID, config.Pattern)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Goroutine %d shutting down: %s", goroutineID, config.Pattern)
			return
		case <-ticker.C:
			pollSensorOnce(config)
		}
	}
}

func pollSensorOnce(sensorTarget SensorConfig) {
	if sensorTarget.Pattern == "/proc/meminfo" {
		pollMeminfo(sensorTarget)
		return
	}

	paths, err := filepath.Glob(sensorTarget.Pattern)
	if err != nil || len(paths) == 0 {
		log.Printf("No paths found for %s", sensorTarget.Pattern)
		return
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Failed to read %s: %v", path, err)
			continue
		}
		valueStr := strings.TrimSpace(string(data))
		valueInt, err := strconv.Atoi(valueStr)
		if err != nil {
			log.Printf("Failed to parse value in %s: %v", path, err)
			continue
		}
		value := float64(valueInt)

		origin := extractOriginFromPath(path, sensorTarget)

		metric := MetricMessage{
			Name:      sensorTarget.Name,
			Help:      sensorTarget.Help,
			Value:     value,
			Labels:    map[string]string{"origin": origin},
			Timestamp: time.Now(),
		}

		sendMetricToKafka(metric)
		log.Printf("Sent %s{origin=%s} = %.2f to Kafka", sensorTarget.Name, origin, value)
	}
}

func pollMeminfo(sensorTarget SensorConfig) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("Failed to read /proc/meminfo: %v", err)
		return
	}

	allowedKeys := map[string]struct{}{
		"MemAvailable": {},
		"MemFree":      {},
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if _, ok := allowedKeys[key]; !ok {
			continue
		}
		valueStr := fields[1]
		valueInt, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		value := valueInt

		metric := MetricMessage{
			Name:      sensorTarget.Name,
			Help:      sensorTarget.Help,
			Value:     value,
			Labels:    map[string]string{"origin": key},
			Timestamp: time.Now(),
		}

		sendMetricToKafka(metric)
		log.Printf("Sent %s{origin=%s} = %.2f to Kafka", sensorTarget.Name, key, value)
	}
}

func extractOriginFromPath(path string, config SensorConfig) string {
	if config.OriginRegex != "" {
		re, err := regexp.Compile(config.OriginRegex)
		if err != nil {
			log.Printf("Invalid regex %q for sensor %s: %v", config.OriginRegex, config.Name, err)
			return "unknown"
		}
		matches := re.FindStringSubmatch(path)
		if len(matches) > 0 {
			return matches[0] // full match
		}
		return "unknown"
	}

	// fallback for compatibility
	if strings.Contains(path, "/cpu") {
		dir := filepath.Dir(path)
		parentDir := filepath.Dir(dir)
		return filepath.Base(parentDir)
	} else if strings.Contains(path, "/thermal_zone") {
		dir := filepath.Dir(path)
		return filepath.Base(dir)
	}
	return "unknown"
}
