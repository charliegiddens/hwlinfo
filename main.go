package main

import (
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

var (
	cpu_tempGauge_avg = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cpu_temperature_celsius_average",
		Help: "Average CPU temperature in Celsius",
	})

	cpu_freqGauge_peak = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cpu_frequency_mhz_peak",
		Help: "Peak CPU clock speed in MHz",
	})
)

type SensorConfig struct {
	Pattern   string
	Gauge     prometheus.Gauge
	isAverage bool
	Divisor   float64
}

var sensor_targets = []SensorConfig{
	{"/sys/class/thermal/thermal_zone*/temp", cpu_tempGauge_avg, true, 1000.0},
	{"/sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq", cpu_freqGauge_peak, false, 1000.0},
}

func main() {

	prometheus.MustRegister(cpu_tempGauge_avg)
	prometheus.MustRegister(cpu_freqGauge_peak)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Println("Prometheus metrics @ :8080/metrics")
		http.ListenAndServe(":8080", nil)
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		gatherSensorData()
	}

}

func gatherSensorData() {
	for _, sensorTarget := range sensor_targets { // Fix syntax
		paths, err := filepath.Glob(sensorTarget.Pattern) // Use the pattern!
		if err != nil || len(paths) == 0 {
			log.Printf("No paths found for %s", sensorTarget.Pattern)
			continue // Don't Fatal, just skip
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
			if sensorTarget.isAverage {
				result := runningTotal / float64(validCount)
				sensorTarget.Gauge.Set(result)
				log.Printf("Average %s: %.1f", sensorTarget.Pattern, result)
			} else {
				sensorTarget.Gauge.Set(maxValue)
				log.Printf("Peak %s: %.0f", sensorTarget.Pattern, maxValue)
			}
		}
	}
}
