package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type SensorConfig struct {
	Pattern     string               `json:"pattern"`
	Name        string               `json:"name"`
	Help        string               `json:"help"`
	OriginRegex string               `json:"origin_regex,omitempty"`
	GaugeVec    *prometheus.GaugeVec `json:"-"` // no serialisation
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

	for i := range config.Sensors {
		config.Sensors[i].GaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: config.Sensors[i].Name,
			Help: config.Sensors[i].Help,
		}, []string{"origin"})
		prometheus.MustRegister(config.Sensors[i].GaugeVec)
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

		sensorTarget.GaugeVec.WithLabelValues(origin).Set(value)
		log.Printf("Set %s{origin=%s} = %.2f", sensorTarget.Name, origin, value)
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

		sensorTarget.GaugeVec.WithLabelValues(key).Set(value)
		log.Printf("Set %s{origin=%s} = %.2f", sensorTarget.Name, key, value)
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
