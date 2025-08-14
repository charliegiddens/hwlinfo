package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricMessage struct {
	Name      string            `json:"name"`
	Help      string            `json:"help"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
	Timestamp time.Time         `json:"timestamp"`
}

type MetricRegistry struct {
	mu      sync.RWMutex
	metrics map[string]*prometheus.GaugeVec
}

func NewMetricRegistry() *MetricRegistry {
	return &MetricRegistry{
		metrics: make(map[string]*prometheus.GaugeVec),
	}
}

func (mr *MetricRegistry) GetOrCreateGauge(name, help string, labelNames []string) *prometheus.GaugeVec {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if gauge, exists := mr.metrics[name]; exists {
		return gauge
	}

	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, labelNames)

	// Register with Prometheus
	err := prometheus.Register(gauge)
	if err != nil {
		// If already registered, try to get the existing one
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existingGauge, ok := are.ExistingCollector.(*prometheus.GaugeVec); ok {
				mr.metrics[name] = existingGauge
				return existingGauge
			}
		}
		log.Printf("Failed to register metric %s: %v", name, err)
		return nil
	}

	mr.metrics[name] = gauge
	return gauge
}

type ConsumerGroupHandler struct {
	registry *MetricRegistry
}

func (h *ConsumerGroupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *ConsumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			var metric MetricMessage
			err := json.Unmarshal(message.Value, &metric)
			if err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				session.MarkMessage(message, "")
				continue
			}

			// Extract label names from the metric labels
			labelNames := make([]string, 0, len(metric.Labels))
			labelValues := make([]string, 0, len(metric.Labels))
			for k, v := range metric.Labels {
				labelNames = append(labelNames, k)
				labelValues = append(labelValues, v)
			}

			// Get or create the gauge
			gauge := h.registry.GetOrCreateGauge(metric.Name, metric.Help, labelNames)
			if gauge != nil {
				gauge.WithLabelValues(labelValues...).Set(metric.Value)
				log.Printf("Updated metric %s{%v} = %.2f", metric.Name, metric.Labels, metric.Value)
			}

			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

func main() {
	registry := NewMetricRegistry()

	binaryPath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	binaryDir := filepath.Dir(binaryPath)
	envPath := filepath.Join(binaryDir, "..", ".env")

	if err := godotenv.Load(envPath); err != nil {
		log.Fatal("Error loading .env file:", err)
	}

	// Initialize Kafka consumer
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	// Get configuration from environment
	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	topic := os.Getenv("KAFKA_TOPIC")
	consumerGroup := os.Getenv("KAFKA_CONSUMER_GROUP")

	// Create consumer group
	consumer, err := sarama.NewConsumerGroup(brokers, consumerGroup, config)
	if err != nil {
		log.Fatal("Failed to create consumer group:", err)
	}
	defer consumer.Close()

	// Start Prometheus metrics server
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Println("Prometheus metrics server started on :8080/metrics")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal("Failed to start metrics server:", err)
		}
	}()

	// Start consuming from Kafka
	ctx := context.Background()
	handler := &ConsumerGroupHandler{registry: registry}

	log.Printf("Starting Kafka consumer for topic: %s, group: %s", topic, consumerGroup)

	for {
		if err := consumer.Consume(ctx, []string{topic}, handler); err != nil {
			log.Printf("Error from consumer: %v", err)
			time.Sleep(time.Second * 5) // Wait before retrying
		}

		// Check if context was cancelled, signaling that the consumer should stop
		if ctx.Err() != nil {
			return
		}
	}
}
