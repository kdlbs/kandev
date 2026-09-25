package models

import "errors"

var (
	ErrKubernetesEnvironmentNotFound = errors.New("kubernetes environment inventory not found")
	ErrKubernetesEnvironmentConflict = errors.New("kubernetes environment ownership or operation changed")
)

// KubernetesEnvironment owns physical resource inventory independently of a session.
// Metadata contains the admitted resource identity and workload snapshot. Secrets
// are encrypted store references and must never be serialized in public responses.
type KubernetesEnvironment struct {
	EnvironmentID       string
	TaskID              string
	OwnershipGeneration int64
	Revision            int64
	OperationID         string
	Metadata            map[string]interface{}
	ControlSecretID     string
	BootstrapSecretID   string
}
