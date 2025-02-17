package tracking

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gonum.org/v1/gonum/stat"
	"grpc-benchmark-study/internal/calculation"
)

// TrackingEntry holds the sent Calculation, the response Calculation,
// a flag indicating if the response was received, and the latency in ms.
type TrackingEntry struct {
	Sent      calculation.Calculation
	Response  calculation.Calculation
	Received  bool
	LatencyMs int64
	SentAt    time.Time // internal field used to compute latency
}

// Tracker holds a map of Calculation.ID to TrackingEntry and a mutex for safe concurrent access.
type Tracker struct {
	mu        sync.Mutex
	data      map[int32]*TrackingEntry
	startTime time.Time
	endTime   time.Time
	duration  time.Duration
}

// LatencyStats holds a summary of latency metrics.
type LatencyStats struct {
	AverageLatency float64 // average latency in ms
	MedianLatency  float64 // 50th percentile in ms
	P90Latency     float64 // 90th percentile in ms
	P95Latency     float64 // 95th percentile in ms
	MaxLatency     int64   // maximum latency in ms
	MinLatency     int64   // minimum latency in ms
	StdDevLatency  float64 // standard deviation in ms
}

// String returns a nicely formatted string representation of the latency stats.
func (ls LatencyStats) String() string {
	return fmt.Sprintf(
		"Latency Summary:\n"+
			"  Average Latency: %.2f ms\n"+
			"  Median Latency: %.2f ms\n"+
			"  90th Percentile: %.2f ms\n"+
			"  95th Percentile: %.2f ms\n"+
			"  Minimum Latency: %d ms\n"+
			"  Maximum Latency: %d ms\n"+
			"  Standard Deviation: %.2f ms",
		ls.AverageLatency, ls.MedianLatency, ls.P90Latency, ls.P95Latency,
		ls.MinLatency, ls.MaxLatency, ls.StdDevLatency)
}

// NewTracker creates and returns a new Tracker.
func NewTracker() *Tracker {
	return &Tracker{
		data: make(map[int32]*TrackingEntry),
	}
}

// AddSent registers a sent Calculation. It records the sent Calculation and the current time.
func (t *Tracker) AddSent(calc calculation.Calculation) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data[calc.ID] = &TrackingEntry{
		Sent:     calc,
		SentAt:   time.Now(),
		Received: false,
	}
}

// RecordResponse records the response Calculation for the given ID and computes the latency.
func (t *Tracker) RecordResponse(response calculation.Calculation) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok := t.data[response.ID]; ok {
		entry.Response = response
		entry.Received = true
		entry.LatencyMs = time.Since(entry.SentAt).Milliseconds()
	}
}

// GetEntry retrieves the tracking entry for a given Calculation.ID.
func (t *Tracker) GetEntry(id int32) (*TrackingEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.data[id]
	return entry, ok
}

// Data returns a copy of the internal tracking map.
func (t *Tracker) Data() map[int32]*TrackingEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	copyMap := make(map[int32]*TrackingEntry, len(t.data))
	for k, v := range t.data {
		copyMap[k] = v
	}
	return copyMap
}

func (t *Tracker) Start() {
	t.mu.Lock()
	t.startTime = time.Now()
	t.mu.Unlock()
}

func (t *Tracker) Stop() {
	t.mu.Lock()
	t.endTime = time.Now()
	t.mu.Unlock()
}

func (t *Tracker) Duration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.endTime.Sub(t.startTime)
}

// LatencySummary computes and returns a summary of latency statistics from all received entries.
func (t *Tracker) LatencySummary() LatencyStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	var latencies []float64
	var sum float64
	var count int64
	var max int64 = 0
	var min int64 = 0

	// Collect latencies from all entries that received a response.
	for _, entry := range t.data {
		if !entry.Received {
			continue
		}
		lat := float64(entry.LatencyMs)
		latencies = append(latencies, lat)
		sum += lat
		count++
		if entry.LatencyMs > max {
			max = entry.LatencyMs
		}
		if min == 0 || entry.LatencyMs < min {
			min = entry.LatencyMs
		}
	}

	var avg float64
	if count > 0 {
		avg = sum / float64(count)
	}

	// Sort latencies to compute percentiles.
	sort.Float64s(latencies)

	var median, p90, p95 float64
	if count > 0 {
		median = stat.Quantile(0.5, stat.Empirical, latencies, nil)
		p90 = stat.Quantile(0.90, stat.Empirical, latencies, nil)
		p95 = stat.Quantile(0.95, stat.Empirical, latencies, nil)
	}

	// Compute standard deviation.
	stdDev := stat.StdDev(latencies, nil)

	return LatencyStats{
		AverageLatency: avg,
		MedianLatency:  median,
		P90Latency:     p90,
		P95Latency:     p95,
		MaxLatency:     max,
		MinLatency:     min,
		StdDevLatency:  stdDev,
	}
}

func (t *Tracker) SentReceivedSummary() string {
	total := len(t.data)
	receivedCount := 0
	for _, entry := range t.data {
		if entry.Received {
			receivedCount++
		}
	}
	return fmt.Sprintf("Total Entries: %d, Received: %d", total, receivedCount)
}
