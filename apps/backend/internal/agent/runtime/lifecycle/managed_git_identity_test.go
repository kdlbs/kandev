package lifecycle

import (
	"net/http"

	"github.com/kandev/kandev/internal/common/subproc"
)

// managedGitIdentityJSON is the /identity body of an agentctl that applies the
// managed Git policy, which the control client requires before creating an instance.
const managedGitIdentityJSON = `{"server_identity":"test","capabilities":["` + subproc.ManagedGitPolicyCapability + `"]}`

func writeManagedGitIdentity(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(managedGitIdentityJSON))
}
