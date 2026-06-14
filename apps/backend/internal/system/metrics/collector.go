package metrics

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Collector struct {
	procRoot   string
	sysRoot    string
	cgroupRoot string
	prevCPU    *cpuTimes
}

func NewCollector() *Collector {
	return &Collector{
		procRoot:   "/proc",
		sysRoot:    "/sys",
		cgroupRoot: "/sys/fs/cgroup",
	}
}

func (c *Collector) Sample(_ context.Context, metricIDs []string, diskPath string) SourceSnapshot {
	if diskPath == "" {
		diskPath = "/"
	}
	samples := make([]MetricSample, 0, len(metricIDs))
	for _, id := range metricIDs {
		samples = append(samples, c.sampleMetric(id, diskPath))
	}
	return SourceSnapshot{
		ID:      "kandev-backend",
		Label:   "Kandev backend",
		Kind:    "backend",
		Metrics: samples,
	}
}

func (c *Collector) sampleMetric(id string, diskPath string) MetricSample {
	switch id {
	case MetricCPUPercent:
		value, err := c.cpuPercent()
		return sample(id, "CPU", "%", value, err)
	case MetricMemoryPercent:
		value, err := c.memoryPercent()
		return sample(id, "Memory", "%", value, err)
	case MetricDiskPercent:
		value, err := diskPercent(diskPath)
		return sample(id, "Disk", "%", value, err)
	case MetricCPUTemp:
		value, err := cpuTemp(c.sysRoot)
		return sample(id, "CPU temp", "C", value, err)
	case MetricIOLoad:
		value, err := ioLoad(c.procRoot)
		return sample(id, "I/O load", "", value, err)
	default:
		return MetricSample{ID: id, Label: id, Available: false, Error: "unknown metric"}
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
	current, err := readProcStat(filepath.Join(c.procRoot, "stat"))
	if err != nil {
		return 0, err
	}
	if c.prevCPU == nil {
		c.prevCPU = &current
		return 0, nil
	}
	value := calculateCPUPercent(*c.prevCPU, current)
	c.prevCPU = &current
	return value, nil
}

func readProcStat(path string) (cpuTimes, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cpuTimes{}, err
	}
	fields := strings.Fields(strings.SplitN(string(data), "\n", 2)[0])
	if len(fields) < 8 || fields[0] != "cpu" {
		return cpuTimes{}, errors.New("invalid /proc/stat cpu line")
	}
	var total uint64
	var idle uint64
	for i, field := range fields[1:] {
		v, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, err
		}
		total += v
		if i == 3 || i == 4 {
			idle += v
		}
	}
	return cpuTimes{total: total, idle: idle}, nil
}

func calculateCPUPercent(prev, current cpuTimes) float64 {
	totalDelta := current.total - prev.total
	if totalDelta == 0 {
		return 0
	}
	idleDelta := current.idle - prev.idle
	return (1 - float64(idleDelta)/float64(totalDelta)) * 100
}

func (c *Collector) memoryPercent() (float64, error) {
	if value, ok := c.cgroupMemoryPercent(); ok {
		return value, nil
	}
	return memInfoPercent(filepath.Join(c.procRoot, "meminfo"))
}

func (c *Collector) cgroupMemoryPercent() (float64, bool) {
	usage, err := readUintFile(filepath.Join(c.cgroupRoot, "memory.current"))
	if err != nil {
		return 0, false
	}
	maxRaw, err := os.ReadFile(filepath.Join(c.cgroupRoot, "memory.max"))
	if err != nil {
		return 0, false
	}
	maxText := strings.TrimSpace(string(maxRaw))
	if maxText == "max" {
		return 0, false
	}
	limit, err := strconv.ParseUint(maxText, 10, 64)
	if err != nil || limit == 0 {
		return 0, false
	}
	return float64(usage) / float64(limit) * 100, true
}

func memInfoPercent(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	values := map[string]uint64{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			values[key] = v
		}
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 {
		return 0, errors.New("MemTotal missing")
	}
	if available == 0 {
		available = values["MemFree"]
	}
	return (1 - float64(available)/float64(total)) * 100, nil
}

func diskPercent(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if total == 0 {
		return 0, errors.New("disk total is zero")
	}
	return (1 - float64(free)/float64(total)) * 100, nil
}

func cpuTemp(sysRoot string) (float64, error) {
	paths, err := filepath.Glob(filepath.Join(sysRoot, "class/thermal/thermal_zone*/temp"))
	if err != nil || len(paths) == 0 {
		return 0, errors.New("cpu temperature unavailable")
	}
	for _, path := range paths {
		raw, err := readUintFile(path)
		if err != nil {
			continue
		}
		value := float64(raw)
		if value > 1000 {
			value /= 1000
		}
		return value, nil
	}
	return 0, errors.New("cpu temperature unavailable")
}

func ioLoad(procRoot string) (float64, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, "loadavg"))
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, errors.New("loadavg missing")
	}
	return strconv.ParseFloat(fields[0], 64)
}

func readUintFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
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
