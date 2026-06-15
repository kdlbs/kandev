//go:build !linux && !darwin && !windows

package metrics

import "errors"

func readCPUTimes(_ *Collector) (cpuTimes, error) {
	return cpuTimes{}, errors.New("cpu metrics unavailable on this platform")
}

func (c *Collector) memoryPercent() (float64, error) {
	return 0, errors.New("memory metrics unavailable on this platform")
}

func (c *Collector) cpuTempValue() (float64, error) {
	return 0, errors.New("cpu temperature unavailable on this platform")
}

func (c *Collector) ioLoadValue() (float64, error) {
	return 0, errors.New("load average unavailable on this platform")
}
