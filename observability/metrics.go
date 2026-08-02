package observability

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Labels map[string]string

type Metrics interface {
	AddCounter(name string, delta float64, labels Labels)
	SetGauge(name string, value float64, labels Labels)
	ObserveHistogram(name string, value float64, labels Labels)
}

type metricsBox struct{ metrics Metrics }

var defaultMetrics atomic.Pointer[metricsBox]

func SetDefaultMetrics(metrics Metrics) {
	if metrics == nil {
		metrics = NewMemoryMetrics()
	}
	defaultMetrics.Store(&metricsBox{metrics: metrics})
}

func DefaultMetrics() Metrics {
	if box := defaultMetrics.Load(); box != nil && box.metrics != nil {
		return box.metrics
	}
	metrics := NewMemoryMetrics()
	SetDefaultMetrics(metrics)
	return metrics
}

type MetricSnapshot struct {
	Counters   map[string]float64      `json:"counters"`
	Gauges     map[string]float64      `json:"gauges"`
	Histograms map[string]Distribution `json:"histograms"`
}

type Distribution struct {
	Count int64   `json:"count"`
	Sum   float64 `json:"sum"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type MemoryMetrics struct {
	mu         sync.RWMutex
	counters   map[string]float64
	gauges     map[string]float64
	histograms map[string]Distribution
}

func NewMemoryMetrics() *MemoryMetrics {
	return &MemoryMetrics{counters: map[string]float64{}, gauges: map[string]float64{}, histograms: map[string]Distribution{}}
}

func (m *MemoryMetrics) AddCounter(name string, delta float64, labels Labels) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.counters[metricKey(name, labels)] += delta
	m.mu.Unlock()
}

func (m *MemoryMetrics) SetGauge(name string, value float64, labels Labels) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.gauges[metricKey(name, labels)] = value
	m.mu.Unlock()
}

func (m *MemoryMetrics) ObserveHistogram(name string, value float64, labels Labels) {
	if m == nil {
		return
	}
	key := metricKey(name, labels)
	m.mu.Lock()
	distribution := m.histograms[key]
	if distribution.Count == 0 || value < distribution.Min {
		distribution.Min = value
	}
	if distribution.Count == 0 || value > distribution.Max {
		distribution.Max = value
	}
	distribution.Count++
	distribution.Sum += value
	m.histograms[key] = distribution
	m.mu.Unlock()
}

func (m *MemoryMetrics) Snapshot() MetricSnapshot {
	result := MetricSnapshot{Counters: map[string]float64{}, Gauges: map[string]float64{}, Histograms: map[string]Distribution{}}
	if m == nil {
		return result
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for key, value := range m.counters {
		result.Counters[key] = value
	}
	for key, value := range m.gauges {
		result.Gauges[key] = value
	}
	for key, value := range m.histograms {
		result.Histograms[key] = value
	}
	return result
}

func metricKey(name string, labels Labels) string {
	name = strings.TrimSpace(name)
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result strings.Builder
	result.WriteString(name)
	result.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			result.WriteByte(',')
		}
		result.WriteString(key)
		result.WriteByte('=')
		result.WriteString(labels[key])
	}
	result.WriteByte('}')
	return result.String()
}
