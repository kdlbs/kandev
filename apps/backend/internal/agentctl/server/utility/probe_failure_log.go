package utility

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	"go.uber.org/zap"
)

// sanitizeProbeDiagnostic bounds provider diagnostics and redacts private data.
func sanitizeProbeDiagnostic(s string) string {
	return securityutil.Sanitize(scrubTmpPaths(redactManagedNPMPaths(strings.TrimSpace(s))))
}

func redactManagedNPMPaths(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "npm error path ") || strings.HasPrefix(lower, "npm err! path ") {
			lines[index] = "npm error path <path>"
			continue
		}
		if strings.HasPrefix(lower, "npm error enoent ") || strings.HasPrefix(lower, "npm err! enoent ") {
			lines[index] = "npm error enoent <redacted>"
			continue
		}
		if strings.HasPrefix(lower, "npm error syscall ") || strings.HasPrefix(lower, "npm err! syscall ") {
			fields := strings.Fields(trimmed)
			if len(fields) == 4 && (strings.EqualFold(fields[3], "open") || strings.EqualFold(fields[3], "read")) {
				lines[index] = "npm error syscall " + strings.ToLower(fields[3])
			} else {
				lines[index] = "npm error diagnostic redacted"
			}
		}
	}
	return strings.Join(lines, "\n")
}

// logProbeFailure puts only sanitized diagnostics in the message retained by
// the JSON relay, which excludes arbitrary structured context.
func (e *ACPInferenceExecutor) logProbeFailure(
	req *ProbeRequest,
	err error,
	stderr string,
	failureCode, managedFailureCode ProbeFailureCode,
) {
	if req.InferenceConfig != nil && !req.InferenceConfig.LaunchWrapper.IsZero() {
		e.logger.Error(fmt.Sprintf("ACP wrapped probe failed: failure_code=%s", failureCode),
			zap.String("agent_id", req.AgentID),
			zap.String("failure_code", string(failureCode)))
		return
	}
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
