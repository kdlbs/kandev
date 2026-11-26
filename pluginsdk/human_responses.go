package pluginsdk

import "context"

// Human response kinds a linked Kandev user can authorize through a plugin.
const (
	HumanResponseKindPermission    = "permission"
	HumanResponseKindClarification = "clarification"
	HumanResponseKindStepApproval  = "step_approval"
)

// ExternalIdentityLinkRequest redeems a link code that a Kandev user created
// on the plugin's settings page. Provider and Subject name the external
// account that presented the code (for example "discord" and a user ID);
// DisplayName is that account's display name, shown to the Kandev user.
type ExternalIdentityLinkRequest struct {
	RequestID, Code, Provider, Subject, DisplayName string
}

// ExternalIdentityLink is one active link from an external account to a
// Kandev user, scoped to the calling plugin installation.
type ExternalIdentityLink struct {
	ID, Provider, Subject, DisplayName string
	UserID, UserDisplayName            string
	CreatedAt                          string // RFC3339
}

// HumanResponseReceiptRequest asks the Host for a single-use receipt for one
// exact response by the Kandev user linked to Provider/Subject. TargetID is
// the interaction ID for permission and clarification responses and the
// session ID for step approval; ExpectedResourceVersion is the target's
// observed version. The payload fields must match the later exact command:
// OptionID or Cancelled for a permission, Answers for a clarification, and
// none for a step approval.
type HumanResponseReceiptRequest struct {
	RequestID, WorkspaceID, Provider, Subject string
	Kind, TargetID, ExpectedResourceVersion   string
	OptionID                                  string
	Cancelled                                 bool
	Answers                                   []ClarificationAnswer
	ApprovalRevision                          uint64
	ManifestDigest                            string
}

// InteractionActor names the Kandev user who decided and the external
// surface that carried the decision, for example "discord".
type InteractionActor struct {
	UserID, DisplayName, Via string
}

// HumanResponseReceipt is a single-use, five-minute proof of one linked
// user's decision, bound to the calling installation, the workspace, the
// target revision, and the exact payload.
type HumanResponseReceipt struct {
	ID, Kind, TargetID, ResourceVersion string
	ExpiresAt                           string // RFC3339
	Actor                               InteractionActor
}

// HumanResponseDelegationManager links external accounts to Kandev users and
// mints receipts for those users. It requires the manifest declaration
// api_write: [human_responses]; receipts also require the workspace grant
// host.v2.write:human_responses.
type HumanResponseDelegationManager interface {
	LinkExternalIdentity(context.Context, ExternalIdentityLinkRequest) (*CommandResult, ExternalIdentityLink, error)
	RequestHumanResponseReceipt(context.Context, HumanResponseReceiptRequest) (*CommandResult, HumanResponseReceipt, error)
}

type HumanResponseDelegationHost interface {
	HumanResponseDelegation() HumanResponseDelegationManager
}

// HostHumanResponseDelegation returns the optional linked-account extension.
func HostHumanResponseDelegation(host Host) (HumanResponseDelegationManager, bool) {
	provider, ok := host.(HumanResponseDelegationHost)
	if !ok {
		return nil, false
	}
	return provider.HumanResponseDelegation(), true
}
