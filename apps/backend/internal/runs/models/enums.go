package models

type RunStatus string

// Run queue status values.
const (
	RunStatusQueued   RunStatus = "queued"
	RunStatusClaimed  RunStatus = "claimed"
	RunStatusFinished RunStatus = "finished"
	RunStatusFailed   RunStatus = "failed"
)

// String implements fmt.Stringer.
func (s RunStatus) String() string { return string(s) }

type RoutingBlockedStatus string

// Routing blocked-status values.
const (
	RoutingBlockedWaitingForCapacity RoutingBlockedStatus = "waiting_for_provider_capacity"
	RoutingBlockedActionRequired     RoutingBlockedStatus = "blocked_provider_action_required"
)

// String implements fmt.Stringer.
func (s RoutingBlockedStatus) String() string { return string(s) }

type RunEventLevel string

// Run event level values.
const (
	RunEventLevelInfo  RunEventLevel = "info"
	RunEventLevelWarn  RunEventLevel = "warn"
	RunEventLevelError RunEventLevel = "error"
)

// String implements fmt.Stringer.
func (l RunEventLevel) String() string { return string(l) }

type RunEventType string

// Well-known run event types. Adapters may emit additional values.
const (
	RunEventTypeInit          RunEventType = "init"
	RunEventTypeAdapterInvoke RunEventType = "adapter.invoke"
	RunEventTypeStep          RunEventType = "step"
	RunEventTypeComplete      RunEventType = "complete"
	RunEventTypeError         RunEventType = "error"
	RunEventTypeRuntimeDenied RunEventType = "runtime.denied"
	RunEventTypeRuntimeAction RunEventType = "runtime.action"
)

// String implements fmt.Stringer.
func (t RunEventType) String() string { return string(t) }
