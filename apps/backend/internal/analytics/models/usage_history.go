package models

// UsagePartition identifies a work-equivalence group before mutable dimensions
// are considered. Both sides of a correction are returned to projection owners.
type UsagePartition struct {
	ScopeID     string
	SubjectID   string
	PartitionID string
	WorkKey     string
}

type UsageMetricSource struct {
	Source      string
	ItemID      string
	ActorID     string
	ActorStatus string
	ActorKind   string
}

// UsageCanonicalFact keeps token and cost provenance and actor evidence separate.
// Incomplete overlap never creates an allocated remainder.
type UsageCanonicalFact struct {
	TokenMetricAbsent       bool `json:"-"`
	CostMetricAbsent        bool `json:"-"`
	NativeAccountingVersion int  `json:"-"`
	Key                     string
	SegmentKey              string
	BundleKey               string
	Partition               UsagePartition
	Measurement             SessionUsageMeasurement
	TokenSource             *UsageMetricSource
	CostSource              *UsageMetricSource
	CoverageKind            string
	TokenSemantics          string
	Partial                 bool
	Reason                  string
}

type UsageSelectionQuery struct {
	Partition UsagePartition
	Cursor    string
	Limit     int
}

type UsageSelectionPage struct {
	Facts       []UsageCanonicalFact
	NextCursor  string
	HasMore     bool
	Preparing   bool
	VisitedRows int
}

// UsageMutation is replay input for the owner of commit ordering and generations.
// The mutation and its invalidations are produced inside the caller's transaction.
type UsageMutation struct {
	CommitSequence int64
	Operation      string
	Source         string
	ItemID         string
	Before         *SessionUsageMeasurement
	After          *SessionUsageMeasurement
	OldPartition   *UsagePartition
	NewPartition   *UsagePartition
	BeforePayload  []byte
	AfterPayload   []byte
}

type UsageProjectionState struct {
	Status     string
	Generation string
	Pending    bool
}
