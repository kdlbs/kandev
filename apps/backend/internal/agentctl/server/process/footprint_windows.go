//go:build windows

package process

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	psapiDLL            = windows.NewLazySystemDLL("psapi.dll")
	kernel32DLL         = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessMem   = psapiDLL.NewProc("GetProcessMemoryInfo")
	procOpenProcess     = kernel32DLL.NewProc("OpenProcess")
	procCloseHandle     = kernel32DLL.NewProc("CloseHandle")
	processMemoryAccess = uint32(0x0010 | 0x0400) // PROCESS_QUERY_INFORMATION | PROCESS_VM_READ
)

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS. PagefileUsage is the
// commit charge; WorkingSetSize is the resident set.
type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var processMemoryCountersSize = uint32(unsafe.Sizeof(processMemoryCounters{}))

// footprintFromProcessTree measures every live descendant of rootPID by
// following parent links out of one Toolhelp process snapshot. A process that
// exits between the snapshot and the measurement is counted as unreadable
// rather than dropped, so the reported count never understates the tree the walk
// actually observed.
func footprintFromProcessTree(rootPID int) ProcessFootprint {
	var footprint ProcessFootprint
	if rootPID <= 0 {
		return footprint
	}
	parentOf, ok := windowsProcessParents()
	if !ok {
		footprint.unreadable()
		return footprint
	}
	for pid := range parentOf {
		if !descendsFromProcess(pid, rootPID, parentOf) {
			continue
		}
		committed, resident, readable := windowsProcessMemory(pid)
		if !readable {
			footprint.unreadable()
			continue
		}
		footprint.add(committed, resident)
	}
	return footprint
}

// windowsProcessParents returns every live process keyed by its own process
// identifier, valued by its parent process identifier. A snapshot that cannot be
// taken reports ok=false so the caller reports nothing rather than an empty
// measurement that would read as "no processes".
func windowsProcessParents() (map[int]int, bool) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	parentOf := make(map[int]int)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, false
	}
	for {
		parentOf[int(entry.ProcessID)] = int(entry.ParentProcessID)
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	if len(parentOf) == 0 {
		return nil, false
	}
	return parentOf, true
}

// windowsProcessMemory reports commit charge and resident bytes for one process.
// It fails closed: an unreadable process reports readable=false rather than
// zeros, so a permissions denial is never presented as "this process is free".
func windowsProcessMemory(pid int) (committed, resident uint64, readable bool) {
	// A successful Call reports Errno(0), which is a non-nil error interface, so
	// the handle and the return value decide success. Testing the error would
	// mark every process unreadable.
	handle, _, _ := procOpenProcess.Call(uintptr(processMemoryAccess), 0, uintptr(pid))
	if handle == 0 {
		return 0, 0, false
	}
	defer func() { _, _, _ = procCloseHandle.Call(handle) }()

	var counters processMemoryCounters
	counters.cb = processMemoryCountersSize
	ret, _, _ := procGetProcessMem.Call(
		handle,
		uintptr(unsafe.Pointer(&counters)),
		unsafe.Sizeof(counters),
	)
	if ret == 0 {
		return 0, 0, false
	}
	return uint64(counters.PagefileUsage), uint64(counters.WorkingSetSize), true
}
