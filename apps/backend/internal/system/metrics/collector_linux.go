//go:build linux

package metrics

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readCPUTimes(c *Collector) (cpuTimes, error) {
	return readProcStat(filepath.Join(c.procRoot, "stat"))
}

func (c *Collector) memoryPercent() (float64, error) {
	if value, ok := c.cgroupMemoryPercent(); ok {
		return value, nil
	}
	return memInfoPercent(filepath.Join(c.procRoot, "meminfo"))
}

func (c *Collector) cpuTempValue() (float64, error) {
	return cpuTemp(c.sysRoot)
}

func (c *Collector) ioLoadValue() (float64, error) {
	return ioLoad(c.procRoot)
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
