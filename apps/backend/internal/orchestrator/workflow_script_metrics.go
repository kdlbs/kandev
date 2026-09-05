package orchestrator

import "expvar"

var (
	workflowScriptStartedTotal  = expvar.NewMap("workflow_script_started_total")
	workflowScriptTerminalTotal = expvar.NewMap("workflow_script_terminal_total")
)

func metricLabel(pairs ...string) string {
	label := ""
	for index := 0; index < len(pairs); index += 2 {
		if index > 0 {
			label += ";"
		}
		label += pairs[index] + "=" + pairs[index+1]
	}
	return label
}
