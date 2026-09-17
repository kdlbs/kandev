package runtime

import (
	"context"
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const healthReady = "ready"

func (s *Service) credentialObservation(ctx context.Context, b *models.AssistantBinding, d models.CredentialDescriptor) models.ContextCredential {
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: b.OwnerUserID, Role: authn.RoleMember})
	validation := models.CredentialValidation{Status: "unavailable", DescriptorRevision: d.Revision, ProfileID: d.ProfileID, Reference: d.Reference, Reason: "resolver_unavailable"}
	profile, err := s.contextProfileRevision(ctx, b.WorkspaceID, d.ProfileID)
	switch {
	case err != nil:
		validation.Reason = "execution_profile_unavailable"
	case d.ExpiresAt != nil && !d.ExpiresAt.After(time.Now()):
		validation.Reason = "descriptor_expired"
	case s.Credentials != nil:
		observed := s.Credentials.CredentialHealth(ctx, b.WorkspaceID, d)
		validation.Status, validation.Reason = credentialHealthReason(observed.Status)
		if observed.ValidatedAt != nil && !observed.ValidatedAt.IsZero() {
			checked := observed.ValidatedAt.UTC().Truncate(time.Second)
			validation.ValidatedAt = &checked
		}
		if validation.Status == healthReady && (validation.ValidatedAt == nil || observed.ConfigurationGeneration == "") {
			validation.Status, validation.Reason = "unknown", "not_checked"
		}
		validation.ConfigurationGeneration = cursorScope("credential-v1", profile, d.Resolver, d.Reference, strconv.FormatInt(d.Revision, 10), observed.ConfigurationGeneration)
	}
	entry := models.ContextCredential{CredentialDescriptor: d, Health: validation.Status, Validation: validation}
	if entry.Health != healthReady {
		entry.UnblockAction = d.UnlockPolicy
	}
	return entry
}

func credentialHealthReason(status string) (string, string) {
	switch status {
	case healthReady:
		return status, "metadata_available"
	case "locked":
		return status, "resolver_locked"
	case "missing":
		return status, "reference_missing"
	case "unavailable":
		return status, "resolver_unavailable"
	default:
		return "unknown", "not_checked"
	}
}
