package streams

import "fmt"

// SessionConfigurationRejectedError marks an explicit provider response that
// refused a session configuration request.
type SessionConfigurationRejectedError struct {
	Err error
}

func (e *SessionConfigurationRejectedError) Error() string {
	if e == nil || e.Err == nil {
		return "session configuration was rejected"
	}
	return e.Err.Error()
}

func (e *SessionConfigurationRejectedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewSessionConfigurationRejectedError(err error) error {
	if err == nil {
		return fmt.Errorf("session configuration was rejected")
	}
	return &SessionConfigurationRejectedError{Err: err}
}

// ConfigurationResult describes the value established by one correlated ACP
// configuration response. The requested value is not evidence of the value
// the agent accepted.
type ConfigurationResult struct {
	ConfigID   string             `json:"config_id,omitempty"`
	Requested  string             `json:"requested"`
	Effective  string             `json:"effective,omitempty"`
	Confirmed  bool               `json:"confirmed"`
	ModelState *SessionModelState `json:"model_state,omitempty"`
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
