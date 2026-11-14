package pluginsdk

import pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"

// UsageSegmentIdentityV1 declares one disjoint member of a source's alternative
// decomposition of retained work. Schema 2 is required when Segment is present.
// IDs are local to the authenticated collector, not provider identity proof.
// At overlapping coverage, different collectors' decompositions are
// alternatives. Proven disjoint intervals remain additive.
// DecompositionID stays fixed across coverage periods and subject bindings.
// SegmentID names an immutable producer record or a documented immutable group
// series. It stays fixed across supported corrections and lifetime/day views.
// A group-defining label change without row identity is not a proven correction:
// the producer must quarantine ambiguous replacement membership, not invent a
// lineage or retire missing groups. Provider labels need not define a member.
// A partial report does not retire missing members; use explicit retirement or
// the existing complete-set protocol when a source can prove complete membership.
type UsageSegmentIdentityV1 struct {
	DecompositionID string
	SegmentID       string
}

func (v *UsageSegmentIdentityV1) toProto() *pluginv1.UsageSegmentIdentityV1 {
	if v == nil {
		return nil
	}
	return &pluginv1.UsageSegmentIdentityV1{DecompositionId: v.DecompositionID, SegmentId: v.SegmentID}
}

func usageSegmentIdentityV1FromProto(v *pluginv1.UsageSegmentIdentityV1) *UsageSegmentIdentityV1 {
	if v == nil {
		return nil
	}
	return &UsageSegmentIdentityV1{DecompositionID: v.GetDecompositionId(), SegmentID: v.GetSegmentId()}
}
