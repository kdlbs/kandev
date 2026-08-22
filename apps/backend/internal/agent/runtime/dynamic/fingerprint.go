package dynamic

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// CredentialBindingDescriptor contains only non-secret binding identity. The
// fingerprint is opaque and safe to persist in route health rows.
type CredentialBindingDescriptor struct {
	Version              int    `json:"version"`
	AgentFamilyID        string `json:"agent_family_id"`
	AuthenticationMethod string `json:"authentication_method"`
	CredentialSourceKind string `json:"credential_source_kind"`
	CredentialLocator    string `json:"credential_locator"`
	ExecutorNamespace    string `json:"executor_namespace"`
	AuthorizationScope   string `json:"authorization_scope"`
	WorkspaceScope       string `json:"workspace_scope"`
}

type BindingFingerprinter struct {
	key []byte
}

// CredentialBindingResolver canonicalizes adapter descriptors before deriving
// an opaque installation-scoped fingerprint. It is intentionally small so
// concrete launch adapters can provide non-secret identity without importing
// the routing engine's storage or circuit types.
type CredentialBindingResolver struct {
	fingerprinter *BindingFingerprinter
}

type InstallationKeyStore interface {
	LoadOrCreate(context.Context) ([]byte, error)
}

func NewPersistentBindingFingerprinter(ctx context.Context, store InstallationKeyStore) (*BindingFingerprinter, error) {
	key, err := store.LoadOrCreate(ctx)
	if err != nil {
		return nil, err
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("installation key store returned an empty key")
	}
	return NewBindingFingerprinter(key), nil
}

func NewPersistentCredentialBindingResolver(ctx context.Context, store InstallationKeyStore) (*CredentialBindingResolver, error) {
	fingerprinter, err := NewPersistentBindingFingerprinter(ctx, store)
	if err != nil {
		return nil, err
	}
	return &CredentialBindingResolver{fingerprinter: fingerprinter}, nil
}

func NewBindingFingerprinter(installationKey []byte) *BindingFingerprinter {
	key := make([]byte, len(installationKey))
	copy(key, installationKey)
	return &BindingFingerprinter{key: key}
}

func NewCredentialBindingResolver(installationKey []byte) *CredentialBindingResolver {
	return &CredentialBindingResolver{fingerprinter: NewBindingFingerprinter(installationKey)}
}

// Fingerprint returns the same key for semantically equivalent descriptors.
// Empty binding identity is rejected so callers can use a profile-scoped
// fallback instead of accidentally sharing health across unknown accounts.
func (r *CredentialBindingResolver) Fingerprint(descriptor CredentialBindingDescriptor) string {
	if r == nil || r.fingerprinter == nil {
		return ""
	}
	descriptor = canonicalDescriptor(descriptor)
	if descriptor.AgentFamilyID == "" || descriptor.AuthenticationMethod == "" ||
		descriptor.CredentialSourceKind == "" {
		return ""
	}
	return r.fingerprinter.Fingerprint(descriptor)
}

// Resolve chooses a conservative profile scope when an adapter cannot prove
// a usable credential binding. The profile ID is never part of a shared
// provider/account fingerprint, only this isolated fallback key.
func (r *CredentialBindingResolver) Resolve(descriptor CredentialBindingDescriptor, profileID string) string {
	if r == nil || r.fingerprinter == nil {
		return "profile:" + profileID
	}
	if fingerprint := r.Fingerprint(descriptor); fingerprint != "" {
		return fingerprint
	}
	return r.fingerprinter.FallbackFingerprint(profileID)
}

func (f *BindingFingerprinter) Fingerprint(descriptor CredentialBindingDescriptor) string {
	descriptor = canonicalDescriptor(descriptor)
	data, err := json.Marshal(descriptor)
	if err != nil || len(f.key) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, f.key)
	_, _ = mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func (f *BindingFingerprinter) FallbackFingerprint(profileID string) string {
	return "profile:" + profileID
}

func canonicalDescriptor(descriptor CredentialBindingDescriptor) CredentialBindingDescriptor {
	descriptor.AgentFamilyID = strings.TrimSpace(strings.ToLower(descriptor.AgentFamilyID))
	descriptor.AuthenticationMethod = strings.TrimSpace(strings.ToLower(descriptor.AuthenticationMethod))
	descriptor.CredentialSourceKind = strings.TrimSpace(strings.ToLower(descriptor.CredentialSourceKind))
	descriptor.CredentialLocator = strings.TrimSpace(descriptor.CredentialLocator)
	descriptor.ExecutorNamespace = strings.TrimSpace(descriptor.ExecutorNamespace)
	descriptor.AuthorizationScope = strings.TrimSpace(descriptor.AuthorizationScope)
	descriptor.WorkspaceScope = strings.TrimSpace(descriptor.WorkspaceScope)
	return descriptor
}
