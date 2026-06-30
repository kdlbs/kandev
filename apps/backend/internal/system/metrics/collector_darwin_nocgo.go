//go:build darwin && !cgo

package metrics

import "errors"

func readCPUTimes(_ *Collector) (cpuTimes, error) {
	return cpuTimes{}, errors.New("cpu metrics unavailable on darwin without cgo")
}

func (c *Collector) memoryPercent() (float64, error) {
	return 0, errors.New("memory metrics unavailable on darwin without cgo")
}

func (c *Collector) cpuTempValue() (float64, error) {
	return 0, errors.New("cpu temperature unavailable on darwin without cgo")
}

func (c *Collector) ioLoadValue() (float64, error) {
	return 0, errors.New("load average unavailable on darwin without cgo")
}
