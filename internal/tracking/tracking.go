package tracking

import (
	"sync"
	"time"

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
	defer t.mu.Unlock()
}

func (t *Tracker) Stop() {
	t.mu.Lock()
	t.endTime = time.Now()
	defer t.mu.Unlock()
}

func (t *Tracker) Duration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.endTime.Sub(t.startTime)
}
