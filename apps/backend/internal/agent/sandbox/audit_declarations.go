package sandbox

import contract "github.com/kandev/kandev/internal/common/sandbox"

func validAuditDeclarations(sources []contract.RuntimeSource, major uint32) bool {
	if len(sources) == 0 {
		return true
	}
	if major != 2 || len(sources) > 8 {
		return false
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if seen[source.Key] || source.Kind != contract.RuntimeSourceAudit && source.Kind != contract.RuntimeSourceAuditFinal || source.ValidateForProtocol(major) != nil {
			return false
		}
		seen[source.Key] = true
	}
	return true
}
