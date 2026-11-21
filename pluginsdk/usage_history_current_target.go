package pluginsdk

import (
	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UsageCollectionCurrentTargetQueryV1 selects at most one current authorized
// target. Every nonempty field is an exact constraint. WorkspaceID is required.
// At least one constraint is required; SnapshotID and Cursor are forbidden.
type UsageCollectionCurrentTargetQueryV1 struct {
	TaskID            string
	SessionID         string
	ExternalSessionID string
	SubjectBinding    string
}

func (v *UsageCollectionCurrentTargetQueryV1) toProto() *pluginv1.UsageCollectionCurrentTargetQueryV1 {
	if v == nil {
		return nil
	}
	return &pluginv1.UsageCollectionCurrentTargetQueryV1{TaskId: v.TaskID, SessionId: v.SessionID, ExternalSessionId: v.ExternalSessionID, SubjectBinding: v.SubjectBinding}
}

func usageCollectionCurrentTargetQueryV1FromProto(v *pluginv1.UsageCollectionCurrentTargetQueryV1) *UsageCollectionCurrentTargetQueryV1 {
	if v == nil {
		return nil
	}
	return &UsageCollectionCurrentTargetQueryV1{TaskID: v.GetTaskId(), SessionID: v.GetSessionId(), ExternalSessionID: v.GetExternalSessionId(), SubjectBinding: v.GetSubjectBinding()}
}

func validateUsageCurrentTargetQuery(query UsageCollectionTargetQueryV1) error {
	point := query.CurrentTarget
	if point == nil {
		return nil
	}
	if query.WorkspaceID == "" || query.SnapshotID != "" || query.Cursor != "" ||
		point.TaskID == "" && point.SessionID == "" && point.ExternalSessionID == "" && point.SubjectBinding == "" {
		return status.Error(codes.InvalidArgument, "current target lookup requires a workspace and exact constraint without a snapshot or cursor")
	}
	return nil
}

func validateUsageCurrentTargetPage(query UsageCollectionTargetQueryV1, page UsageCollectionTargetPageV1) error {
	if query.CurrentTarget == nil {
		return nil
	}
	if !page.CurrentTarget {
		return status.Error(codes.Unimplemented, "the Host does not acknowledge current target lookup")
	}
	if len(page.Targets) > 1 || page.SnapshotID != "" || page.SnapshotUpperBound != 0 || page.NextCursor != "" || page.HasMore || page.Preparing {
		return status.Error(codes.FailedPrecondition, "the Host returned enumeration state for a current target lookup")
	}
	return nil
}
