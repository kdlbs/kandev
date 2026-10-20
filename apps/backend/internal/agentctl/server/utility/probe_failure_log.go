package utility

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	"go.uber.org/zap"
)

// sanitizeProbeDiagnostic bounds provider diagnostics and redacts private data.
func sanitizeProbeDiagnostic(s string) string {
	return securityutil.Sanitize(scrubTmpPaths(strings.TrimSpace(s)))
}

// logProbeFailure puts only sanitized diagnostics in the message retained by
// the JSON relay, which excludes arbitrary structured context.
func (e *ACPInferenceExecutor) logProbeFailure(
	req *ProbeRequest,
	err error,
	stderr string,
	failureCode, managedFailureCode ProbeFailureCode,
) {
	message := "ACP probe failed"
	if req.ProfileContext {
		message = "ACP profile probe failed"
	}
	safeError := sanitizeProbeDiagnostic(err.Error())
	safeStderr := sanitizeProbeDiagnostic(stderr)
	e.logger.Error(fmt.Sprintf("%s: %s; failure_code=%s; managed_failure_code=%s; stderr=%s",
		message, safeError, failureCode, managedFailureCode, safeStderr),
		zap.String("agent_id", req.AgentID),
		zap.String("error", safeError),
		zap.String("failure_code", string(failureCode)),
		zap.String("managed_failure_code", string(managedFailureCode)),
		zap.String("stderr", safeStderr))
}
