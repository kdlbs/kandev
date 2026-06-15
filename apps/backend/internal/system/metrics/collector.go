package metrics

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

type Collector struct {
	procRoot   string
	sysRoot    string
	cgroupRoot string
	prevCPU    *cpuTimes
	lastCPUAt  time.Time
	mu         sync.Mutex
}

func NewCollector() *Collector {
	return &Collector{
		procRoot:   "/proc",
		sysRoot:    "/sys",
		cgroupRoot: "/sys/fs/cgroup",
	}
}

func (c *Collector) Sample(ctx context.Context, metricIDs []string, diskPath string) SourceSnapshot {
	if err := ctx.Err(); err != nil {
		return canceledBackendSnapshot(metricIDs, err)
	}
	for !c.mu.TryLock() {
		select {
		case <-ctx.Done():
			return canceledBackendSnapshot(metricIDs, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer c.mu.Unlock()
	if diskPath == "" {
		diskPath = "/"
	}
	samples := make([]MetricSample, 0, len(metricIDs))
	for _, id := range metricIDs {
		if err := ctx.Err(); err != nil {
			samples = append(samples, sample(id, metricLabel(id), metricUnit(id), 0, err))
			continue
		}
		samples = append(samples, c.sampleMetric(ctx, id, diskPath))
	}
	return SourceSnapshot{
		ID:      "kandev-backend",
		Label:   "Kandev backend",
		Kind:    "backend",
		Metrics: samples,
	}
}

func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prevCPU = nil
	c.lastCPUAt = time.Time{}
}

func (c *Collector) sampleMetric(ctx context.Context, id string, diskPath string) MetricSample {
	switch id {
	case MetricCPUPercent:
		value, err := c.cpuPercent()
		return sample(id, "CPU", "%", value, err)
	case MetricMemoryPercent:
		value, err := c.memoryPercent()
		return sample(id, "Memory", "%", value, err)
	case MetricDiskPercent:
		value, err := diskPercent(ctx, diskPath)
		return sample(id, "Disk", "%", value, err)
	case MetricCPUTemp:
		value, err := c.cpuTempValue()
		return sample(id, "CPU temp", "C", value, err)
	case MetricIOLoad:
		value, err := c.ioLoadValue()
		return sample(id, "Load avg", "", value, err)
	default:
		return MetricSample{ID: id, Label: id, Available: false, Error: "unknown metric"}
	}
}

func canceledBackendSnapshot(metricIDs []string, err error) SourceSnapshot {
	samples := make([]MetricSample, 0, len(metricIDs))
	for _, id := range metricIDs {
		samples = append(samples, sample(id, metricLabel(id), metricUnit(id), 0, err))
	}
	return SourceSnapshot{
		ID:      "kandev-backend",
		Label:   "Kandev backend",
		Kind:    "backend",
		Metrics: samples,
	}
}

func metricLabel(id string) string {
	switch id {
	case MetricCPUPercent:
		return "CPU"
	case MetricMemoryPercent:
		return "Memory"
	case MetricDiskPercent:
		return "Disk"
	case MetricCPUTemp:
		return "CPU temp"
	case MetricIOLoad:
		return "Load avg"
	default:
		return id
	}
}

func metricUnit(id string) string {
	switch id {
	case MetricCPUPercent, MetricMemoryPercent, MetricDiskPercent:
		return "%"
	case MetricCPUTemp:
		return "C"
	default:
		return ""
	}
}

func sample(id, label, unit string, value float64, err error) MetricSample {
	if err != nil {
		return MetricSample{ID: id, Label: label, Unit: unit, Available: false, Error: err.Error()}
	}
	rounded := math.Round(value*10) / 10
	return MetricSample{ID: id, Label: label, Unit: unit, Value: &rounded, Available: true}
}

type cpuTimes struct {
	total uint64
	idle  uint64
}

func (c *Collector) cpuPercent() (float64, error) {
	current, err := readCPUTimes(c)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	if c.prevCPU != nil && !c.lastCPUAt.IsZero() && now.Sub(c.lastCPUAt) > 2*time.Duration(MaxIntervalSeconds)*time.Second {
		c.prevCPU = nil
	}
	if c.prevCPU == nil {
		c.prevCPU = &current
		c.lastCPUAt = now
		return 0, nil
	}
	value := calculateCPUPercent(*c.prevCPU, current)
	c.prevCPU = &current
	c.lastCPUAt = now
	return value, nil
}

func calculateCPUPercent(prev, current cpuTimes) float64 {
	totalDelta := current.total - prev.total
	if totalDelta == 0 {
		return 0
	}
	idleDelta := current.idle - prev.idle
	return (1 - float64(idleDelta)/float64(totalDelta)) * 100
}

func (c *Collector) SampleWithTimestamp(ctx context.Context, settings GlobalSettings) Snapshot {
	source := c.Sample(ctx, settings.Metrics, settings.BackendDiskPath)
	return Snapshot{
		Timestamp:       time.Now().UTC(),
		IntervalSeconds: settings.IntervalSeconds,
		Sources:         []SourceSnapshot{source},
	}
}

func unavailableExecutionSource(id, label, kind, errText string) SourceSnapshot {
	return SourceSnapshot{
		ID:      id,
		Label:   label,
		Kind:    kind,
		Metrics: []MetricSample{{ID: "error", Label: "Metrics", Available: false, Error: fmt.Sprintf("execution metrics unavailable: %s", errText)}},
	}
}
