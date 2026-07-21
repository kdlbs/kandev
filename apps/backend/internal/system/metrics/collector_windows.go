//go:build windows

package metrics

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memoryStatusEx mirrors MEMORYSTATUSEX from windows.h. The first field is the
// struct length in bytes and must be set by the caller before invoking
// GlobalMemoryStatusEx.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// readCPUTimes uses GetSystemTimes. On Windows kernelTime includes idleTime,
// so total non-idle = (kernel + user) - idle and we return total = kernel + user.
func readCPUTimes(_ *Collector) (cpuTimes, error) {
	var idle, kernel, user windows.Filetime
	r1, _, e := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r1 == 0 {
		return cpuTimes{}, fmt.Errorf("GetSystemTimes: %w", e)
	}
	return cpuTimes{
		total: filetimeToUint64(kernel) + filetimeToUint64(user),
		idle:  filetimeToUint64(idle),
	}, nil
}

func filetimeToUint64(ft windows.Filetime) uint64 {
	return (uint64(ft.HighDateTime) << 32) | uint64(ft.LowDateTime)
}

func (c *Collector) memoryPercent() (float64, error) {
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	r1, _, e := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if r1 == 0 {
		return 0, fmt.Errorf("GlobalMemoryStatusEx: %w", e)
	}
	if status.TotalPhys == 0 {
		return 0, errors.New("total physical memory is zero")
	}
	used := status.TotalPhys - status.AvailPhys
	return float64(used) / float64(status.TotalPhys) * 100, nil
}

func (c *Collector) cpuTempValue() (float64, error) {
	return 0, errors.New("cpu temperature unavailable on windows")
}

func (c *Collector) ioLoadValue() (float64, error) {
	return 0, errors.New("load average unavailable on windows")
}
