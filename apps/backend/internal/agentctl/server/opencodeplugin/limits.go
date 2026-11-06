package opencodeplugin

import "time"

const (
	// ProtocolVersion is the current bootstrap and wire contract.
	ProtocolVersion = 1

	// BehaviorShellOutcomes is the only production behavior in this release.
	BehaviorShellOutcomes = "shell-outcomes"
	// ShellOutcomesVersion is the reviewed shell behavior version.
	ShellOutcomesVersion = 1

	// MaxEventRequestBytes bounds one event batch body.
	MaxEventRequestBytes = 64 << 10
	// MaxEventsPerRequest bounds observations in one batch.
	MaxEventsPerRequest = 64
	// MaxClaims bounds the owned shell claim set for one launch.
	MaxClaims = 256
	// MaxIdentityBytes bounds a stable event identity.
	MaxIdentityBytes = 256
	// MaxHelloRequestBytes bounds the hello body.
	MaxHelloRequestBytes = 16 << 10

	// ConfigPollTimeout is the long-poll ceiling.
	ConfigPollTimeout = 15 * time.Second
	// RequestTimeout bounds ordinary event/hello requests.
	RequestTimeout = 5 * time.Second
	// ReadinessBudget bounds the handshake after the plugin first contacts the
	// host. CLI boot is not part of this budget.
	ReadinessBudget = 5 * time.Second
	// BootBudget bounds how long a prepared launch waits for the plugin's first
	// callback before the integration is disabled for that process.
	BootBudget = 30 * time.Second
)
