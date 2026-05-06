package service

import (
	"context"
	"errors"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// GetTaskContext composes the office task-handoffs DTO returned by
// GET /api/v1/tasks/:id/context (phase 7). The DTO is consumed by both
// the prompt builder (phase 8.1) and the task detail React panel (phase
// 8.2) so the data layout stays in one place.
//
// Document bodies are intentionally NOT returned — AvailableDocs lists
// keys + titles only. Agents must explicitly fetch a document via
// get_task_document_kandev (phase 2) and the UI uses the existing
// document endpoint.
func (s *HandoffService) GetTaskContext(ctx context.Context, taskID string) (*v1.TaskContext, error) {
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	if s.tasks == nil {
		return nil, errors.New("task repo not configured")
	}
	related, err := s.ListRelated(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := &v1.TaskContext{
		Task:      toV1Ref(related.Task),
		Children:  toV1Refs(related.Children),
		Siblings:  toV1Refs(related.Siblings),
		Blockers:  toV1Refs(related.Blockers),
		BlockedBy: toV1Refs(related.BlockedBy),
	}
	if related.Parent != nil {
		ref := toV1Ref(*related.Parent)
		out.Parent = &ref
	}
	out.AvailableDocs = s.collectAvailableDocs(ctx, related)
	out.WorkspaceMode, out.WorkspaceGroup = s.resolveWorkspaceFields(ctx, taskID)
	out.BlockedReason = s.resolveBlockedReason(out)
	out.WorkspaceStatus = resolveWorkspaceStatus(out.WorkspaceGroup)
	return out, nil
}

// collectAvailableDocs aggregates documents from the task itself plus
// every relation an agent can read under the phase 2 access rules:
// parent, siblings, AND blockers (the simplified Phase 3 model). The
// blocker branch is critical for end-to-end handoffs — when a consumer
// task wakes after its producer's blocker resolves, the prompt MUST
// name the producer's documents or the agent has no way to discover
// them. Children are excluded so downstream tasks don't crowd the
// surface.
func (s *HandoffService) collectAvailableDocs(ctx context.Context, related *RelatedTasks) []v1.DocumentRef {
	if s.docsRepo == nil {
		return nil
	}
	out := []v1.DocumentRef{}
	seen := map[string]bool{related.Task.ID: true}
	out = appendDocsForRef(ctx, s, related.Task, out)
	if related.Parent != nil {
		seen[related.Parent.ID] = true
		out = appendDocsForRef(ctx, s, *related.Parent, out)
	}
	for _, sib := range related.Siblings {
		if sib != nil && !seen[sib.ID] {
			seen[sib.ID] = true
			out = appendDocsForRef(ctx, s, *sib, out)
		}
	}
	// Blocker docs surface in the available list so the agent woken
	// by task_blockers_resolved knows what to fetch from the producer.
	for _, b := range related.Blockers {
		if b != nil && !seen[b.ID] {
			seen[b.ID] = true
			out = appendDocsForRef(ctx, s, *b, out)
		}
	}
	return out
}

func appendDocsForRef(ctx context.Context, s *HandoffService, ref RelatedTask, out []v1.DocumentRef) []v1.DocumentRef {
	docs, err := s.docsRepo.ListDocuments(ctx, ref.ID)
	if err != nil {
		return out
	}
	for _, d := range docs {
		out = append(out, v1.DocumentRef{
			TaskRef:   toV1Ref(ref),
			Key:       d.Key,
			Title:     d.Title,
			Type:      d.Type,
			SizeBytes: d.SizeBytes,
			UpdatedAt: d.UpdatedAt,
		})
	}
	return out
}

// resolveWorkspaceFields reads task.metadata.workspace.mode + the active
// workspace group for the task. Either may be empty.
func (s *HandoffService) resolveWorkspaceFields(ctx context.Context, taskID string) (string, *v1.WorkspaceGroupRef) {
	t, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || t == nil {
		return "", nil
	}
	mode := taskWorkspaceMode(t.Metadata)
	if s.wsGroups == nil {
		return mode, nil
	}
	g, _ := s.wsGroups.GetWorkspaceGroupForTask(ctx, taskID)
	if g == nil {
		return mode, nil
	}
	groupRef := &v1.WorkspaceGroupRef{
		ID:               g.ID,
		MaterializedPath: g.MaterializedPath,
		MaterializedKind: g.MaterializedKind,
		CleanupStatus:    g.CleanupStatus,
		OwnedByKandev:    g.OwnedByKandev,
	}
	members, _ := s.wsGroups.ListActiveWorkspaceGroupMembers(ctx, g.ID)
	for _, m := range members {
		mt, err := s.tasks.GetTask(ctx, m.TaskID)
		if err != nil || mt == nil {
			continue
		}
		groupRef.Members = append(groupRef.Members, taskToV1Ref(mt))
	}
	return mode, groupRef
}

// resolveBlockedReason picks the most informative reason a task is
// currently blocked from launching. Document-handoff readiness is
// expressed through normal blockers (phase 3), so the surface here is
// "blockers pending" + "workspace restoring".
func (s *HandoffService) resolveBlockedReason(ctx *v1.TaskContext) string {
	if len(ctx.Blockers) > 0 {
		return v1.TaskBlockedReasonBlockersPending
	}
	if ctx.WorkspaceGroup != nil && ctx.WorkspaceGroup.CleanupStatus == orchmodels.WorkspaceCleanupStatusCleaned {
		return v1.TaskBlockedReasonWorkspaceRestoring
	}
	return ""
}

func resolveWorkspaceStatus(g *v1.WorkspaceGroupRef) string {
	if g == nil {
		return v1.TaskWorkspaceStatusActive
	}
	if g.CleanupStatus == orchmodels.WorkspaceCleanupStatusFailed {
		return v1.TaskWorkspaceStatusRequiresConf
	}
	return v1.TaskWorkspaceStatusActive
}

func taskWorkspaceMode(meta map[string]interface{}) string {
	ws, ok := meta["workspace"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := ws["mode"].(string)
	return v
}

func toV1Ref(rt RelatedTask) v1.TaskRef {
	return v1.TaskRef{
		ID:            rt.ID,
		Identifier:    rt.Identifier,
		Title:         rt.Title,
		State:         rt.State,
		WorkspaceID:   rt.WorkspaceID,
		ParentID:      rt.ParentID,
		AssigneeLabel: rt.AssigneeLabel,
		DocumentKeys:  append([]string(nil), rt.DocumentKeys...),
	}
}

func toV1Refs(in []*RelatedTask) []v1.TaskRef {
	if len(in) == 0 {
		return []v1.TaskRef{}
	}
	out := make([]v1.TaskRef, 0, len(in))
	for _, r := range in {
		if r != nil {
			out = append(out, toV1Ref(*r))
		}
	}
	return out
}

func taskToV1Ref(t *models.Task) v1.TaskRef {
	return v1.TaskRef{
		ID:            t.ID,
		Identifier:    t.Identifier,
		Title:         t.Title,
		State:         string(t.State),
		WorkspaceID:   t.WorkspaceID,
		ParentID:      t.ParentID,
		AssigneeLabel: t.AssigneeAgentProfileID,
	}
}
