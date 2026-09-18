package backendapp

import shared "github.com/kandev/kandev/internal/orchestration/models"

const (
	frictionOriginNative           = "native"
	frictionOriginAuthentication   = "authentication"
	frictionAuthenticationRequired = "authentication_required"
	frictionOperationPermission    = "permission"
)

// Code provenance is native; provider classifier internals remain unknown.
func frictionCauseForError(code string) *shared.FrictionCause {
	row := &shared.FrictionCause{Cause: code, Origin: "unknown", Operation: "launch"}
	switch code {
	case "permission_denied_by_user":
		row.Origin, row.Operation, row.Reason = frictionOriginNative, frictionOperationPermission, "denied_authority"
	case "auth_required", frictionAuthenticationRequired:
		row.Origin, row.Reason = frictionOriginAuthentication, frictionAuthenticationRequired
	case "authentication_expired", "token_expired":
		row.Origin, row.Reason = frictionOriginAuthentication, "expired_authentication"
	case "missing_credentials", "provider_not_configured", "subscription_required", "model_unavailable":
		row.Origin, row.Reason = "configuration", "missing_capability"
	case "network_unavailable", "provider_unavailable", "provider_overloaded", "rate_limited":
		row.Origin, row.Reason = "provider", "transport_failure"
	case "agent_transport_lost":
		row.Origin, row.Reason = frictionOriginNative, "transport_failure"
	case "task_error", "repo_error":
		row.Reason = "task_defect"
	case "policy_boundary", "suspected_classifier_error":
		row.Reason = code
	default:
		return nil
	}
	return row
}
