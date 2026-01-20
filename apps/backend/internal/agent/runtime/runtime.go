// Package runtime defines the agent runtime types shared across lifecycle and policy logic.
package runtime

// Name identifies the execution runtime.
type Name string

const (
	NameUnknown    Name = ""
	NameDocker     Name = "docker"
	NameStandalone Name = "standalone"
	NameLocal      Name = "local"
)
