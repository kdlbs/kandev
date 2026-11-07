package usage

import "errors"

// ExecutionUsageDiagnostic exposes only a fixed operation stage and HTTP status.
// Provider response bodies, credential paths and transport errors stay private.
type ExecutionUsageDiagnostic struct {
	Stage      string `json:"stage"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type executionAccountError struct {
	diagnostic ExecutionUsageDiagnostic
	cause      error
}

func (e *executionAccountError) Error() string {
	return "execution usage account " + e.diagnostic.Stage + " failed"
}
func (e *executionAccountError) Unwrap() error { return e.cause }

func accountFailure(stage string, status int, cause error) error {
	return &executionAccountError{diagnostic: ExecutionUsageDiagnostic{Stage: stage, HTTPStatus: status}, cause: cause}
}

// ExecutionUsageErrorDiagnostic never stringifies an untrusted error.
func ExecutionUsageErrorDiagnostic(err error) *ExecutionUsageDiagnostic {
	var account *executionAccountError
	if !errors.As(err, &account) {
		return nil
	}
	copy := account.diagnostic
	return &copy
}
