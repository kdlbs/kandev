package plugins

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/sysprompt"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// sessionToDTO maps a TaskSession to the Go-native Session DTO, resolving
// ACPSessionID via resolveACPSessionID's metadata-then-executors_running
// fallback.
func (h *pluginHost) sessionToDTO(ctx context.Context, s *taskmodels.TaskSession) pluginsdk.Session {
	return pluginsdk.Session{
		ID:               s.ID,
		TaskID:           s.TaskID,
		AgentProfileID:   s.AgentProfileID,
		AgentDisplayName: sessionSnapshotString(s.AgentProfileSnapshot, "agent_display_name"),
		AgentProfileName: sessionSnapshotString(s.AgentProfileSnapshot, "name"),
		Model:            sessionSnapshotModel(s.AgentProfileSnapshot),
		ACPSessionID:     h.resolveACPSessionID(ctx, s),
		State:            string(s.State),
		StartedAt:        s.StartedAt.UTC().Format(time.RFC3339),
		EndedAt:          timePtrToRFC3339(s.CompletedAt),
	}
}

// resolveACPSessionID replicates the source agent-stats plugin's join key
// (docs/decisions/0043-plugin-host-data-api.md, "A real plugin exposed the
// gap"): the agent CLI's own session UUID at
// TaskSession.Metadata["acp"]["session_id"], populated once the agent emits
// a session_info frame. executors_running.resume_token carries the same id
// and survives on sessions that never got that far, so it is a best-effort
// fallback — a lookup failure (including "no such row") is silently treated
// as "no id available" rather than failing the whole read.
func (h *pluginHost) resolveACPSessionID(ctx context.Context, s *taskmodels.TaskSession) string {
	if id := acpSessionIDFromMetadata(s.Metadata); id != "" {
		return id
	}
	running, err := h.taskData.GetExecutorRunningBySessionID(ctx, s.ID)
	if err != nil || running == nil {
		return ""
	}
	return running.ResumeToken
}

func acpSessionIDFromMetadata(metadata map[string]any) string {
	acp, ok := metadata["acp"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := acp["session_id"].(string)
	return id
}

func sessionSnapshotString(snapshot map[string]any, key string) string {
	if snapshot == nil {
		return ""
	}
	v, _ := snapshot[key].(string)
	return v
}

// sessionSnapshotModel mirrors the source plugin's fallback chain for the
// agent-profile snapshot's model field, which has varied key names across
// agent types over time.
func sessionSnapshotModel(snapshot map[string]any) string {
	for _, key := range []string{"model", "model_name", "llm"} {
		if v := sessionSnapshotString(snapshot, key); v != "" {
			return v
		}
	}
	return ""
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Internal model → pluginsdk DTO mapping ──────────────────────────────

func taskModelToDTO(t *taskmodels.Task) pluginsdk.Task {
	repos := make([]pluginsdk.TaskRepository, len(t.Repositories))
	for i, r := range t.Repositories {
		repos[i] = pluginsdk.TaskRepository{
			ID:             r.ID,
			RepositoryID:   r.RepositoryID,
			BaseBranch:     r.BaseBranch,
			Position:       int32(r.Position),
			CheckoutBranch: r.CheckoutBranch,
		}
	}
	return pluginsdk.Task{
		ID:          t.ID,
		WorkspaceID: t.WorkspaceID,
		WorkflowID:  t.WorkflowID,
		Title:       t.Title,
		Description: t.Description,
		State:       string(t.State),
		Priority:    t.Priority,
		// CreatedBy: kandev's Task model has no creating-user column — Origin
		// ("manual"/"agent_created"/"routine"/"automation_run") is the
		// closest analogue and is what this surfaces.
		CreatedBy: t.Origin,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.UTC().Format(time.RFC3339),
		// StartedAt/CompletedAt: the Task model has no started_at/completed_at
		// columns (ArchivedAt is a different concept); left nil in v1.
		ParentID:     stringPtrOrNil(t.ParentID),
		Identifier:   t.Identifier,
		IsEphemeral:  t.IsEphemeral,
		Repositories: repos,
		Metadata:     t.Metadata,

		ArchivedAt:             timePtrToRFC3339(t.ArchivedAt),
		WorkflowStepID:         t.WorkflowStepID,
		Position:               int32(t.Position),
		AssigneeAgentProfileID: t.AssigneeAgentProfileID,
		Labels:                 decodeTaskLabels(t.Labels),
		Autopilot:              t.Autopilot,
		WIPAdmitted:            t.WIPAdmitted,
		QueuedForStepID:        t.QueuedForStepID,
		QueuedAt:               timePtrToRFC3339(t.QueuedAt),
		ProjectID:              t.ProjectID,
		ExternalID:             t.ExternalID,
	}
}

func timePtrToRFC3339(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// decodeTaskLabels turns the model's JSON-array-string into a real slice.
//
// A MALFORMED VALUE YIELDS NO LABELS, NOT AN ERROR. Labels are decoration on a
// read that must not fail because one row holds something unparseable; the
// alternative is a whole task list refused over a cosmetic field.
func decodeTaskLabels(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// taskPRsToDTOs maps kandev's own PR rows onto the plugin contract. Review and
// check state come along because kandev's PR watcher already syncs them: a
// plugin asking "what can merge" would otherwise re-query the forge per pull
// request, which is slower and rate-limited.
func taskPRsToDTOs(prs []*githubsvc.TaskPR) []pluginsdk.TaskPullRequest {
	ordered := make([]*githubsvc.TaskPR, 0, len(prs))
	for _, pr := range prs {
		if pr != nil {
			ordered = append(ordered, pr)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.After(ordered[j].CreatedAt)
	})

	out := make([]pluginsdk.TaskPullRequest, 0, len(ordered))
	for _, pr := range ordered {
		out = append(out, pluginsdk.TaskPullRequest{
			Number:     int64(pr.PRNumber),
			URL:        pr.PRURL,
			Title:      pr.PRTitle,
			State:      pr.State,
			HeadBranch: pr.HeadBranch,
			BaseBranch: pr.BaseBranch,
			// IsDraft is nullable on the row (unknown before the first sync);
			// nil reads as not-draft, matching how kandev's own UI treats it.
			IsDraft: pr.IsDraft != nil && *pr.IsDraft,
			// Only github writes these rows today; the field exists so a future
			// gitlab-backed row is distinguishable rather than mislabelled.
			Provider:                "github",
			MergedAt:                timePtrToRFC3339(pr.MergedAt),
			ClosedAt:                timePtrToRFC3339(pr.ClosedAt),
			ReviewState:             pr.ReviewState,
			ChecksState:             pr.ChecksState,
			MergeableState:          pr.MergeableState,
			UnresolvedReviewThreads: int32(pr.UnresolvedReviewThreads),
			ChecksTotal:             int32(pr.ChecksTotal),
			ChecksPassing:           int32(pr.ChecksPassing),
			Additions:               int32(pr.Additions),
			Deletions:               int32(pr.Deletions),
			AuthorLogin:             pr.AuthorLogin,
		})
	}
	return out
}

func workspaceModelToDTO(w *taskmodels.Workspace) pluginsdk.Workspace {
	return pluginsdk.Workspace{
		ID:                    w.ID,
		Name:                  w.Name,
		Description:           stringPtrOrNil(w.Description),
		OwnerID:               w.OwnerID,
		DefaultExecutorID:     w.DefaultExecutorID,
		DefaultAgentProfileID: w.DefaultAgentProfileID,
		CreatedAt:             w.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:             w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func workflowModelToDTO(w *taskmodels.Workflow) pluginsdk.Workflow {
	return pluginsdk.Workflow{
		ID:          w.ID,
		WorkspaceID: w.WorkspaceID,
		Name:        w.Name,
		Description: stringPtrOrNil(w.Description),
		SortOrder:   int32(w.SortOrder),
		CreatedAt:   w.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   w.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func workflowStepModelToDTO(s *wfmodels.WorkflowStep) pluginsdk.WorkflowStep {
	return pluginsdk.WorkflowStep{
		ID:                 s.ID,
		WorkflowID:         s.WorkflowID,
		Name:               s.Name,
		Position:           int32(s.Position),
		StageType:          string(s.StageType),
		Color:              s.Color,
		IsStartStep:        s.IsStartStep,
		WIPLimit:           int32(s.WIPLimit),
		AgentProfileID:     s.AgentProfileID,
		OnEnterActionTypes: onEnterActionTypesToDTO(s.Events.OnEnter),
	}
}

// onEnterActionTypesToDTO exposes only the on_enter action TYPES, order
// preserved as authored. Action Config (target task ids, payloads, participant
// roles, agent profile ids) is deliberately not exposed to plugins.
func onEnterActionTypesToDTO(actions []wfmodels.OnEnterAction) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, len(actions))
	for i, action := range actions {
		out[i] = string(action.Type)
	}
	return out
}

func repositoryModelToDTO(r *taskmodels.Repository) pluginsdk.Repository {
	return pluginsdk.Repository{
		ID:                   r.ID,
		WorkspaceID:          r.WorkspaceID,
		Name:                 r.Name,
		DefaultBranch:        stringPtrOrNil(r.DefaultBranch),
		SourceType:           r.SourceType,
		ProviderID:           r.Provider,
		ProviderRepositoryID: r.ProviderRepoID,
		ProviderHost:         r.ProviderHost,
		ProviderScope:        r.ProviderScope,
		OwnerOrProject:       r.ProviderOwner,
		ProviderName:         r.ProviderName,
		RemoteURL:            credentialFreeRepositoryURL(r.RemoteURL),
	}
}

func credentialFreeRepositoryURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.User = nil
	return parsed.String()
}

func agentProfileDTOToSDK(p agentsettingsdto.AgentProfileDTO) pluginsdk.AgentProfile {
	return pluginsdk.AgentProfile{
		ID:          p.ID,
		AgentID:     p.AgentID,
		DisplayName: p.AgentDisplayName,
		Name:        p.Name,
		Model:       p.Model,
		Mode:        p.Mode,
	}
}

// messageModelToDTO maps a stored message to the Go-native Message DTO. It
// strips kandev-injected <kandev-system> blocks from content via
// sysprompt.StripSystemContent — the same sanitization the message.added bus
// event applies — so a plugin never sees raw system prompts. Type defaults to
// "message" (matching Message.ToAPI and the repository) when empty.
func messageModelToDTO(m *taskmodels.Message) pluginsdk.Message {
	msgType := string(m.Type)
	if msgType == "" {
		msgType = string(taskmodels.MessageTypeMessage)
	}
	return pluginsdk.Message{
		ID:         m.ID,
		SessionID:  m.TaskSessionID,
		TaskID:     m.TaskID,
		TurnID:     m.TurnID,
		AuthorType: string(m.AuthorType),
		Content:    sysprompt.StripSystemContent(m.Content),
		Type:       msgType,
		CreatedAt:  m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// sessionCodeStatsModelToDTO converts the internal analytics model - which
// represents "capture wasn't active yet for this session" as a nil
// LinesAddedCommitted/LinesDeletedCommitted pair - into the public plugin
// SDK's DTO, which represents the same fact as CommittedLinesAvailable ==
// false with both fields reading 0 (see pluginsdk.SessionCodeStats' doc for
// why the SDK doesn't use pointers here).
func sessionCodeStatsModelToDTO(s *analyticsmodels.SessionCodeStats) pluginsdk.SessionCodeStats {
	dto := pluginsdk.SessionCodeStats{
		SessionID:               s.SessionID,
		LinesAddedPeakPending:   s.LinesAddedPeakPending,
		LinesDeletedPeakPending: s.LinesDeletedPeakPending,
	}
	if s.LinesAddedCommitted != nil && s.LinesDeletedCommitted != nil {
		dto.LinesAddedCommitted = *s.LinesAddedCommitted
		dto.LinesDeletedCommitted = *s.LinesDeletedCommitted
		dto.CommittedLinesAvailable = true
	}
	return dto
}

// interactionModelToDTO converts the assembled durable interaction into the
// public plugin DTO. Timestamps become RFC3339 UTC strings like every other
// Host DTO. The permission title is the agent's prompt text, which the
// message row stores as its content — it is agent-authored, not a
// kandev-injected system block, so it needs no sanitization pass.
func interactionModelToDTO(i *taskmodels.Interaction) pluginsdk.Interaction {
	return pluginsdk.Interaction{
		ID:                i.ID,
		Kind:              string(i.Kind),
		TaskID:            i.TaskID,
		SessionID:         i.SessionID,
		TurnID:            i.TurnID,
		Status:            string(i.Status),
		Title:             i.Title,
		Context:           i.Context,
		ToolCallID:        i.ToolCallID,
		ActionType:        i.ActionType,
		Options:           interactionOptionsToDTOs(i.Options),
		Questions:         interactionQuestionsToDTOs(i.Questions),
		AgentDisconnected: i.AgentDisconnected,
		CreatedAt:         i.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         i.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func interactionOptionsToDTOs(options []taskmodels.InteractionOption) []pluginsdk.InteractionOption {
	if len(options) == 0 {
		return nil
	}
	out := make([]pluginsdk.InteractionOption, len(options))
	for i, option := range options {
		out[i] = pluginsdk.InteractionOption{
			OptionID:    option.ID,
			Label:       option.Label,
			Kind:        option.Kind,
			Description: option.Description,
		}
	}
	return out
}

func interactionQuestionsToDTOs(questions []taskmodels.InteractionQuestion) []pluginsdk.InteractionQuestion {
	if len(questions) == 0 {
		return nil
	}
	out := make([]pluginsdk.InteractionQuestion, len(questions))
	for i, question := range questions {
		out[i] = pluginsdk.InteractionQuestion{
			ID:      question.ID,
			Title:   question.Title,
			Prompt:  question.Prompt,
			Options: interactionOptionsToDTOs(question.Options),
		}
	}
	return out
}
