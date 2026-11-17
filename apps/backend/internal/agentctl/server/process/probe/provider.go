package probe

import "time"

// ProviderWorkloadEnvKey identifies provider descendants independently of the
// task workspace. It is an activity marker and grants no tool authority.
const ProviderWorkloadEnvKey = "KANDEV_PROVIDER_WORKLOAD_ID"

type providerEnvironmentReader interface {
	environmentReader
	HasProviderWorkloadID(int, string) (bool, error)
}

type providerProbeReader struct {
	processTableReader
	providerEnvironmentReader
}

func (r providerProbeReader) HasSessionID(pid int, id string) (bool, error) {
	return r.HasProviderWorkloadID(pid, id)
}

// ProbeProviderBackgroundWorkloads preserves descendant and orphan checks while
// excluding workspace processes that share only the task session ID. Platforms
// without complete provider environment evidence cannot authorize replacement.
func ProbeProviderBackgroundWorkloads(agentPID int, turnStart time.Time, owner string) (Result, error) {
	reader := platformProcessTableReader()
	env, ok := reader.(providerEnvironmentReader)
	if !ok || owner == "" || turnStart.IsZero() {
		return ResultUnknown, nil
	}
	return probeWithReader(providerProbeReader{processTableReader: reader, providerEnvironmentReader: env}, agentPID, turnStart, owner)
}
