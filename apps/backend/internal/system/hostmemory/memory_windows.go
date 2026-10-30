//go:build windows

package hostmemory

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	// GetPerformanceInfo lives in psapi.dll, not kernel32.dll.
	procGetPerformanceInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetPerformanceInfo")
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

// performanceInformation mirrors PERFORMANCE_INFORMATION from psapi.h. The
// counter fields are page counts; Cb must be set to the struct size.
type performanceInformation struct {
	Cb                uint32
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

type windowsReader struct{}

// NewReader returns the Windows host-memory reader.
func NewReader() Reader { return windowsReader{} }

func (windowsReader) Read(ctx context.Context) (Reading, error) {
	if err := ctx.Err(); err != nil {
		return Reading{}, err
	}
	reading, err := readPhysicalMemory()
	if err != nil {
		return Reading{Complete: false}, err
	}
	reading.Complete = true
	if used, limit, ok := readCommitCharge(); ok {
		reading.CommitUsedBytes = used
		reading.CommitLimitBytes = limit
		reading.CommitKnown = true
	}
	return reading, nil
}

func readPhysicalMemory() (Reading, error) {
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	r1, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if r1 == 0 {
		return Reading{}, fmt.Errorf("GlobalMemoryStatusEx: %w", callErr)
	}
	if status.TotalPhys == 0 {
		return Reading{}, errors.New("GlobalMemoryStatusEx reported zero total physical memory")
	}
	return Reading{TotalBytes: status.TotalPhys, AvailableBytes: status.AvailPhys}, nil
}

// readCommitCharge reads the system-wide commit charge. GetPerformanceInfo is
// used deliberately: GlobalMemoryStatusEx's page-file fields are scoped to the
// current process, not the whole system, so they cannot stand in for it.
func readCommitCharge() (used, limit uint64, ok bool) {
	if err := procGetPerformanceInfo.Find(); err != nil {
		// Fail open for this dimension: the available-memory floor still
		// applies, and the gate's commit condition is simply not judged.
		return 0, 0, false
	}
	var info performanceInformation
	info.Cb = uint32(unsafe.Sizeof(info))
	r1, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&info)), uintptr(info.Cb))
	if r1 == 0 || info.PageSize == 0 {
		return 0, 0, false
	}
	return uint64(info.CommitTotal) * uint64(info.PageSize),
		uint64(info.CommitLimit) * uint64(info.PageSize), true
}
