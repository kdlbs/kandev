package plugins

// SSOProvider is a login option surfaced on the pre-auth login screen,
// contributed by an active auth-capable plugin's manifest auth_providers.
// InitiateURL is the plugin webhook the login button navigates to, which
// 302-redirects the browser to the IdP.
type SSOProvider struct {
	ID          string
	DisplayName string
	InitiateURL string
}

// SSOProviders returns the external login options from every active,
// auth-capable plugin that declares auth_providers. A provider whose Initiate
// does not match one of the plugin's declared webhook keys is skipped (its
// button would 404), mirroring the webhook handler's own key check.
func (s *Service) SSOProviders() []SSOProvider {
	var out []SSOProvider
	for _, rec := range s.List() {
		if rec.Status != StatusActive || !rec.Capabilities.Auth {
			continue
		}
		for _, p := range rec.AuthProviders {
			if p.ID == "" || p.Initiate == "" || !manifestDeclaresWebhookKey(rec, p.Initiate) {
				continue
			}
			out = append(out, SSOProvider{
				ID:          p.ID,
				DisplayName: p.DisplayName,
				InitiateURL: "/api/plugins/" + rec.ID + "/webhooks/" + p.Initiate,
			})
		}
	}
	return out
}
