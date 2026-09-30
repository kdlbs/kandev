package agents

// RuntimeUpdateNotice identifies one preference-aware runtime occurrence.
type RuntimeUpdateNotice struct {
	AgentID         string
	RuntimeID       string
	DisplayName     string
	PreviousVersion string
	Version         string
	Status          string
	OccurrenceID    string
	URL             string
}
