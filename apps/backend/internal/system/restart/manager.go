package restart

import (
	"context"
	"errors"
)

type Capability struct {
	Supported bool                   `json:"supported"`
	Mode      string                 `json:"mode"`
	Adapter   string                 `json:"adapter"`
	Reason    string                 `json:"reason,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type RestartResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
}

type Manager interface {
	Capability(ctx context.Context) Capability
	RequestRestart(ctx context.Context) (RestartResponse, error)
}

const (
	AdapterUnsupported = "unsupported"
	AdapterSupervisor  = "supervisor"

	ModeManual     = "manual"
	ModeSupervisor = "supervisor"

	unsupportedReason = "Automatic restart is not available for this launch mode. Restart Kandev from the terminal or service manager."
)

var ErrUnsupported = errors.New("restart unsupported")
