package githubauth

// CredentialBrokerRequest is the managed credential helper's request body and
// the payload the backend /resolve and /reissue routes bind. Multi-scope
// helpers decode KANDEV_GITHUB_CREDENTIAL_SCOPES entries into this type; the
// executor tier writes that variable through its own githubCredentialScope
// struct. The JSON names are therefore a persisted format: never rename or
// alias them.
type CredentialBrokerRequest struct {
	Lease             string `json:"lease,omitempty"`
	ReissueCapability string `json:"reissue_capability,omitempty"`
	TaskID            string `json:"task_id"`
	SessionID         string `json:"session_id"`
	RepositoryID      string `json:"repository_id"`
	Owner             string `json:"owner"`
	Repo              string `json:"repo"`
	Host              string `json:"host"`
	Path              string `json:"path,omitempty"`
	ProviderID        string `json:"provider_id,omitempty"`
	ParentProviderID  string `json:"parent_provider_id,omitempty"`
}
