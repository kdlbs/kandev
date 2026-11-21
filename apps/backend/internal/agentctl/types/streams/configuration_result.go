package streams

// ConfigurationResult describes the value established by one correlated ACP
// configuration response. The requested value is not evidence of the value
// the agent accepted.
type ConfigurationResult struct {
	ConfigID  string `json:"config_id,omitempty"`
	Requested string `json:"requested"`
	Effective string `json:"effective,omitempty"`
	Confirmed bool   `json:"confirmed"`
}

// ConfigurationCommandResult is an ephemeral observation attached to the
// live completion of a recognized ACP configuration command.
type ConfigurationCommandResult struct {
	Status    string `json:"status"`
	ConfigID  string `json:"config_id,omitempty"`
	Requested string `json:"requested"`
	Effective string `json:"effective,omitempty"`
	Confirmed bool   `json:"confirmed"`
}
