//go:build darwin

package metrics

/*
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <mach/host_info.h>
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// readCPUTimes uses Mach host_statistics(HOST_CPU_LOAD_INFO). Darwin does not
// expose kern.cp_time the way BSDs do, so we go straight to Mach. The returned
// cpu_ticks array has CPU_STATE_MAX entries (USER, SYSTEM, IDLE, NICE) of
// natural_t (uint32). Wraparound is rare at 100 Hz (~497 days) and the delta
// arithmetic in calculateCPUPercent already tolerates it via uint64 promotion.
func readCPUTimes(_ *Collector) (cpuTimes, error) {
	var info C.host_cpu_load_info_data_t
	count := C.mach_msg_type_number_t(C.HOST_CPU_LOAD_INFO_COUNT)
	rc := C.host_statistics(
		C.mach_host_self(),
		C.HOST_CPU_LOAD_INFO,
		C.host_info_t(unsafe.Pointer(&info)),
		&count,
	)
	if rc != C.KERN_SUCCESS {
		return cpuTimes{}, fmt.Errorf("host_statistics HOST_CPU_LOAD_INFO: kern_return %d", int(rc))
	}
	var total, idle uint64
	for i := 0; i < int(C.CPU_STATE_MAX); i++ {
		v := uint64(info.cpu_ticks[i])
		total += v
		if i == int(C.CPU_STATE_IDLE) {
			idle = v
		}
	}
	return cpuTimes{total: total, idle: idle}, nil
}

func (c *Collector) memoryPercent() (float64, error) {
	return darwinMemoryPercent()
}

func (c *Collector) cpuTempValue() (float64, error) {
	return 0, errors.New("cpu temperature unavailable on darwin")
}

func (c *Collector) ioLoadValue() (float64, error) {
	return darwinLoadAvg1()
}

// vm.loadavg returns struct loadavg { fixed_pt_t ldavg[3]; long fscale; }.
// On darwin amd64/arm64 long is 8 bytes; the struct is padded to 24 bytes.
func darwinLoadAvg1() (float64, error) {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil {
		return 0, fmt.Errorf("sysctl vm.loadavg: %w", err)
	}
	if len(raw) < 24 {
		return 0, errors.New("sysctl vm.loadavg response too short")
	}
	ld0 := binary.LittleEndian.Uint32(raw[0:4])
	fscale := binary.LittleEndian.Uint64(raw[16:24])
	if fscale == 0 {
		return 0, errors.New("sysctl vm.loadavg fscale is zero")
	}
	return float64(ld0) / float64(fscale), nil
}

// darwinMemoryPercent treats (free + inactive + speculative) pages as
// available, matching Activity Monitor's "Memory Used / Total" approximation.
// Wired, active, and compressor pages count as used.
func darwinMemoryPercent() (float64, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, fmt.Errorf("sysctl hw.memsize: %w", err)
	}
	if total == 0 {
		return 0, errors.New("hw.memsize is zero")
	}
	pageSize := uint64(os.Getpagesize())
	if pageSize == 0 {
		return 0, errors.New("page size is zero")
	}
	free, inactive, speculative, err := readVMStatistics64()
	if err != nil {
		return 0, err
	}
	available := (free + inactive + speculative) * pageSize
	if available > total {
		available = total
	}
	return (1.0 - float64(available)/float64(total)) * 100.0, nil
}

func readVMStatistics64() (free, inactive, speculative uint64, err error) {
	var vm C.vm_statistics64_data_t
	count := C.mach_msg_type_number_t(C.HOST_VM_INFO64_COUNT)
	rc := C.host_statistics64(
		C.mach_host_self(),
		C.HOST_VM_INFO64,
		C.host_info64_t(unsafe.Pointer(&vm)),
		&count,
	)
	if rc != C.KERN_SUCCESS {
		return 0, 0, 0, fmt.Errorf("host_statistics64 HOST_VM_INFO64: kern_return %d", int(rc))
	}
	return uint64(vm.free_count), uint64(vm.inactive_count), uint64(vm.speculative_count), nil
}
