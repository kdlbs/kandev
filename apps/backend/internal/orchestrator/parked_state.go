package orchestrator

import "context"

// backgroundProbePort is the orchestrator's narrow view of the BackgroundProbe
// port defined in the lifecycle package. Using a local interface instead of
// importing lifecycle directly keeps the dependency direction correct and lets
// tests inject a stub without the full lifecycle stack.
type backgroundProbePort interface {
	ProbeBackgroundWorkloads(ctx context.Context, kandevSessionID string) (string, error)
}
