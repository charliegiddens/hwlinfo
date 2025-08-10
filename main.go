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

var tempGauge = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "cpu_temperature_celsius_average",
	Help: "Average CPU temperature in Celsius",
})

func main() {

	prometheus.MustRegister(tempGauge)

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
	paths, err := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	if err != nil {
		log.Fatal(err)
	}

	runningTotal := 0.0

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Failed to read %s: %v", path, err)
			continue
		}

		tempStr := strings.TrimSpace(string(data))
		tempInt, err := strconv.Atoi(tempStr)
		if err != nil {
			log.Printf("Failed to parse temp in %s: %v", path, err)
			continue
		}
		tempC := float64(tempInt) / 1000.0
		// log.Printf("Temperature at %s: %.1fC", path, tempC)

		runningTotal += tempC
	}
	averageTemp := runningTotal / float64(len(paths))

	timeNow := time.Now().Format(time.StampMilli)
	log.Printf("["+timeNow+"] Average temperature: %.1fC", averageTemp)
	tempGauge.Set(averageTemp)
}
