package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
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

	var runningTotal float64

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

	timeNow := time.Now().Format(time.RFC850)
	log.Printf("["+timeNow+"] Average temperature: %.1fC", averageTemp)
}
