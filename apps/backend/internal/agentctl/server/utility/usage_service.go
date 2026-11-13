package utility

import "github.com/kandev/kandev/internal/agent/usage"

// Called by server composition before request handling starts. Every preview
// still proves its own final source before reading this existing account cache.
func (h *Handler) SetExecutionUsageService(service *usage.UsageService) {
	if service != nil {
		if executor, ok := h.acpExecutor.(*ACPInferenceExecutor); ok {
			executor.executionUsage = service
		}
	}
}
