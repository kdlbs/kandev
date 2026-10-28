package hostutility

import "github.com/kandev/kandev/internal/common/subproc"

// managedGitIdentityJSON is the /identity body of an agentctl that applies the
// managed Git policy, which the control client requires before creating an instance.
const managedGitIdentityJSON = `{"server_identity":"test","capabilities":["` + subproc.ManagedGitPolicyCapability + `"]}`
