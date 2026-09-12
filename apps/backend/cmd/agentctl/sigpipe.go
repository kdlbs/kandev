package main

import (
	"os"
	"syscall"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// applySIGPIPEDisposition decides whether this process keeps running after
// the backend that spawned it goes away.
//
// The launcher always pipes this process's stdout/stderr back to the
// spawning backend for logging (internal/agent/runtime/agentctl/launcher's
// pipeOutput). When that backend exits, the kernel closes its end of those
// pipes, and Go's default SIGPIPE disposition kills this process the next
// time it writes to fd 1 or 2, which every subsequent log line does. An
// inherited pipe is therefore one of the ways a departing backend takes its
// agentctl with it.
//
// Which behavior is correct depends on the capability. With agent survival
// enabled the process is meant to outlive a graceful restart, so the write
// must degrade to an ordinary error, which the logger already tolerates.
// With the capability disabled nothing may outlive the backend, so this kill
// path stays intact alongside the others.
//
// Called after configuration is resolved: a process that dies before then
// dies on the default disposition, and the short-lived CLI and
// credential-helper invocations dispatched before configuration keep the
// ordinary disposition a command-line tool should have.
func applySIGPIPEDisposition(cfg *config.Config, ignore func(...os.Signal)) {
	if !cfg.AgentSurvivalEnabled {
		return
	}
	ignore(syscall.SIGPIPE)
}
