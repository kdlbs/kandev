package share

import "github.com/kandev/kandev/internal/common/redaction"

// Preserve the snapshot API while allowing repositories to reuse the redactor
// without importing sharing backends (and their task-service dependencies).
type Redactor = redaction.Redactor

const (
	RuleAbsPath         = redaction.RuleAbsPath
	RuleSecretSK        = redaction.RuleSecretSK
	RuleSecretGHP       = redaction.RuleSecretGHP
	RuleSecretGHO       = redaction.RuleSecretGHO
	RuleSecretGitHubPAT = redaction.RuleSecretGitHubPAT
	RuleSecretAWS       = redaction.RuleSecretAWS
	RuleEnvFile         = redaction.RuleEnvFile
	RuleEnvVars         = redaction.RuleEnvVars

	redactedPlaceholder = "[redacted]"
	envFilePlaceholder  = "[redacted: .env contents]"
)

func NewRedactor(workspaceRoots ...string) *Redactor {
	return redaction.NewRedactor(workspaceRoots...)
}
