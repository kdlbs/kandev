package models

import "time"

// CredentialDescriptor is knowledge of a credential, never the credential itself.
// ProfileID pins the permitted execution account; Scope narrows its consumers.
type CredentialDescriptor struct {
	ID           string     `json:"id"`
	BindingID    string     `json:"binding_id"`
	Revision     int64      `json:"revision"`
	ProfileID    string     `json:"profile_id"`
	Scope        string     `json:"scope"`
	ScopeID      string     `json:"scope_id"`
	Resolver     string     `json:"resolver"`
	Reference    string     `json:"reference"`
	Purpose      string     `json:"purpose"`
	Account      string     `json:"account"`
	Environment  string     `json:"environment"`
	Fields       []string   `json:"fields"`
	UnlockPolicy string     `json:"unlock_policy"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

type ContextCredential struct {
	CredentialDescriptor
	Health        string `json:"health"`
	UnblockAction string `json:"unblock_action,omitempty"`
}

func (d CredentialDescriptor) Matches(b *AssistantBinding, scope ContextScope) bool {
	if d.BindingID != b.ID || d.ProfileID != scope.ProfileID {
		return false
	}
	if d.ExpiresAt != nil && !d.ExpiresAt.After(time.Now()) {
		return false
	}
	m := AgentMemory{Scope: d.Scope, ScopeID: d.ScopeID, OwnerUserID: b.OwnerUserID}
	return m.MatchesContext(b, scope)
}
