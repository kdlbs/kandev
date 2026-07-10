package oslimits

import "context"

// Probe samples a single OS resource limit category.
type Probe interface {
	Name() string
	Category() string
	// Samples returns one or more resource samples for this probe.
	// Returning multiple allows a single probe to cover related metrics
	// (e.g. inotify instances AND inotify watches from one /proc scan).
	Samples(ctx context.Context) ([]Sample, error)
}

// Sample describes the current state of one OS resource limit.
type Sample struct {
	ID           string     // unique ID, e.g. "inotify_instances"
	Name         string     // human label, e.g. "Inotify instances"
	Unit         string     // e.g. "instances"
	Used         uint64     // current usage
	Limit        uint64     // system limit
	UsageRatio   float64    // Used / Limit (0–1)
	Supported    bool       // false on non-Linux platforms
	TopConsumers []Consumer // top N processes by usage, sorted descending
}

// Consumer describes one process's share of a resource.
type Consumer struct {
	PID        int
	Command    string // e.g. "fish", "node"
	FDCount    uint64 // number of inotify fds (for inotify probe)
	WatchCount uint64 // number of watches inside those fds
}
