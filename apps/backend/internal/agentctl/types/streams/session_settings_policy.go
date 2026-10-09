package streams

import "context"

// SessionSettingsPolicy identifies the authority for mode and model settings
// reports produced while loading an existing provider conversation.
type SessionSettingsPolicy string

const (
	SessionSettingsPolicyStrict           SessionSettingsPolicy = "strict"
	SessionSettingsPolicyProviderRestored SessionSettingsPolicy = "provider_restored"
)

type sessionSettingsPolicyContextKey struct{}

// WithSessionSettingsPolicy carries a host-selected policy through a session
// load. Provider reports cannot set this context value themselves.
func WithSessionSettingsPolicy(ctx context.Context, policy SessionSettingsPolicy) context.Context {
	return context.WithValue(ctx, sessionSettingsPolicyContextKey{}, policy)
}

// SessionSettingsPolicyFromContext returns the host-selected session load
// policy, defaulting to strict for existing callers.
func SessionSettingsPolicyFromContext(ctx context.Context) SessionSettingsPolicy {
	if ctx == nil {
		return SessionSettingsPolicyStrict
	}
	policy, _ := ctx.Value(sessionSettingsPolicyContextKey{}).(SessionSettingsPolicy)
	if policy == "" {
		return SessionSettingsPolicyStrict
	}
	return policy
}
