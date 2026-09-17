package runtime

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/auth/authn"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantCredentialValidationMetadata(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	s.Credentials = syntheticCredentialHealth("ready")
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/credentials/example"
	request := map[string]any{"resolver": "kandev", "reference": "example-reference", "purpose": "Example service", "profile_id": "personal", "account": "example-account", "scope": "workspace", "fields": []string{"password"}, "unlock_policy": "Unlock the example resolver", "expected_revision": 0}
	require.Equal(t, 200, runtimeRequest(t, router, "PUT", path, "", "", request).Code)
	response := runtimeRequest(t, router, "GET", path, "", "", nil)
	require.Equal(t, 200, response.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotNil(t, body["validation"])
	validation := body["validation"].(map[string]any)
	require.Equal(t, "ready", validation["status"])
	require.EqualValues(t, 1, validation["descriptor_revision"])
	require.Equal(t, "personal", validation["profile_id"])
	require.Equal(t, "example-reference", validation["reference"])
	require.NotEmpty(t, validation["configuration_generation"])
	require.NotEmpty(t, validation["validated_at"])
	request["validation"] = map[string]any{"status": "ready", "validated_at": "2099-01-01T00:00:00Z"}
	require.Equal(t, 422, runtimeRequest(t, router, "PUT", path, "", "", request).Code)
	delete(request, "validation")
	packetURL := "/api/v1/orchestration/runtime/context/" + goal + "?profile_id=personal"
	agent := assistantRuntimeRouter(s)
	first := runtimeRequest(t, agent, "GET", packetURL, token, run, nil)
	require.Equal(t, 200, first.Code, first.Body.String())
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &packet))
	b, err := s.Repo.AssistantBinding(context.Background(), "owner")
	require.NoError(t, err)
	_, err = s.currentPacket(context.Background(), b, packet.ID)
	require.NoError(t, err)
	s.Credentials = syntheticCredentialHealth("locked")
	_, err = s.currentPacket(context.Background(), b, packet.ID)
	require.ErrorContains(t, err, "context changed")
	for _, state := range []string{"unknown", "missing", "locked", "unavailable"} {
		s.Credentials = syntheticCredentialHealth(state)
		response = runtimeRequest(t, router, "GET", path, "", "", nil)
		require.Equal(t, 200, response.Code)
		require.Contains(t, response.Body.String(), `"status":"`+state+`"`)
	}
}

type metadataCredentialCheck struct {
	observed models.CredentialValidation
	owner    string
	calls    int
}

func (r *metadataCredentialCheck) CredentialHealth(ctx context.Context, _ string, _ models.CredentialDescriptor) models.CredentialValidation {
	identity, _ := authn.IdentityFromContext(ctx)
	r.owner = identity.UserID
	r.calls++
	return r.observed
}

func TestAssistantCredentialValidationRechecksWithoutTimestampChurn(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	b, err := s.Repo.AssistantBinding(context.Background(), "owner")
	require.NoError(t, err)
	d := models.CredentialDescriptor{ID: "reference", BindingID: b.ID, ProfileID: "personal", Scope: "workspace", ScopeID: "ws", Resolver: "kandev", Reference: "example-reference", Purpose: "Example service", Account: "sample", Fields: []string{"password"}, UnlockPolicy: "Ask owner"}
	require.NoError(t, s.Repo.SaveCredentialDescriptor(context.Background(), &d, 0))
	checked := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	reader := &metadataCredentialCheck{observed: models.CredentialValidation{Status: "ready", ValidatedAt: &checked, ConfigurationGeneration: "one"}}
	s.Credentials = reader
	path := "/api/v1/orchestration/runtime/context/" + goal + "?profile_id=personal"
	response := runtimeRequest(t, assistantRuntimeRouter(s), "GET", path, token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var p models.ContextPacket
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &p))
	checked = checked.Add(time.Hour)
	current, err := s.currentPacket(context.Background(), b, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ID, current.ID)
	require.Equal(t, "owner", reader.owner)
	require.Equal(t, 2, reader.calls)
	require.NotEqual(t, p.Credentials[0].Validation.ValidatedAt, current.Credentials[0].Validation.ValidatedAt)
	reader.observed.ConfigurationGeneration = "two"
	_, err = s.currentPacket(context.Background(), b, p.ID)
	require.ErrorContains(t, err, "context changed")
	reader.observed.ConfigurationGeneration = "one"
	profile, err := s.Personas.Profiles.GetAgentProfile(context.Background(), "personal")
	require.NoError(t, err)
	profile.Name = "Updated example profile"
	require.NoError(t, s.Personas.Profiles.UpdateAgentProfile(context.Background(), profile))
	_, err = s.currentPacket(context.Background(), b, p.ID)
	require.ErrorContains(t, err, "context changed")
}
