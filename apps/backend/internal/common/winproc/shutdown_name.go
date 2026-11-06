package winproc

import "fmt"

// shutdownEventName is the per-process event name shared by the Go launcher,
// the backend, and the desktop shell (apps/desktop/src-tauri/src/windows_shutdown.rs).
// It stays untagged so every platform compiles and tests the contract.
func shutdownEventName(pid int) string {
	return fmt.Sprintf(`Local\KandevShutdown-%d`, pid)
}
