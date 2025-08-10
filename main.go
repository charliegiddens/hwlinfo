package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type SensorConfig struct {
	Pattern   string           `json:"pattern"`
	Name      string           `json:"name"`
	Help      string           `json:"help"`
	IsAverage bool             `json:"is_average"`
	Divisor   float64          `json:"divisor"`
	Gauge     prometheus.Gauge `json:"-"` // no serialisation
}

type SensorFile struct {
	Sensors []SensorConfig `json:"sensors"`
}

var sensor_targets []SensorConfig

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

	// prometheus gauge creation
	for i := range config.Sensors {
		config.Sensors[i].Gauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name: config.Sensors[i].Name,
			Help: config.Sensors[i].Help,
		})
		prometheus.MustRegister(config.Sensors[i].Gauge)
	}

	sensor_targets = config.Sensors
	return nil
}

func main() {

	err := loadSensorConfig("sensor_targets.json")
	if err != nil {
		log.Fatal("Failed to load sensor config:", err)
	}

	log.Printf("Starting monitoring with %d sensor types", len(sensor_targets))

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Println("Prometheus metrics @ :8080/metrics")
		http.ListenAndServe(":8080", nil)
	}()

	// context for clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start goroutine foreach sensor type
	for i, sensorTarget := range sensor_targets {
		go pollSensorContinuously(ctx, sensorTarget, i+1)
	}

	// Keep main thread waiting forever to stop defer ending main()
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
	paths, err := filepath.Glob(sensorTarget.Pattern)
	if err != nil || len(paths) == 0 {
		log.Printf("No paths found for %s", sensorTarget.Pattern)
		return
	}

	runningTotal := 0.0
	maxValue := 0.0
	validCount := 0

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
		value := float64(valueInt) / sensorTarget.Divisor
		runningTotal += value
		if value > maxValue {
			maxValue = value
		}
		validCount++
	}

	if validCount > 0 {
		if sensorTarget.IsAverage {
			result := runningTotal / float64(validCount)
			sensorTarget.Gauge.Set(result)
			log.Printf("Average %s: %.1f", sensorTarget.Pattern, result)
		} else {
			sensorTarget.Gauge.Set(maxValue)
			log.Printf("Peak %s: %.0f", sensorTarget.Pattern, maxValue)
		}
	}
}
