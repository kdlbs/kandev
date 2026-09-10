package lifecycle

import "expvar"

// agentActiveRuntimes is a process-wide gauge of executions currently owned
// by the lifecycle runtime. It carries no task/session/execution labels.
var agentActiveRuntimes = expvar.NewInt("agent_active_runtimes")
