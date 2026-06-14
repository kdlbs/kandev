package restart

import "context"

type UnsupportedManager struct {
	Reason string
}

func NewUnsupportedManager(reason string) *UnsupportedManager {
	return &UnsupportedManager{Reason: reason}
}

func (m *UnsupportedManager) Capability(context.Context) Capability {
	return Capability{
		Supported: false,
		Mode:      ModeManual,
		Adapter:   AdapterUnsupported,
		Reason:    m.reason(),
	}
}

func (m *UnsupportedManager) RequestRestart(context.Context) (RestartResponse, error) {
	return RestartResponse{
		Accepted: false,
		Message:  m.reason(),
	}, ErrUnsupported
}

func (m *UnsupportedManager) reason() string {
	if m != nil && m.Reason != "" {
		return m.Reason
	}
	return unsupportedReason
}
