package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/admission"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func (r *messageAddSwitchRepo) CreateMessageWithInitialTaskBrief(
	_ context.Context,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
) error {
	r.messagesMu.Lock()
	defer r.messagesMu.Unlock()
	if len(r.messages) == 0 {
		message.Content = candidate.Content
		candidate.Selected = true
	} else {
		candidate.Selected = false
	}
	message.PromptIndex = len(r.messages) + 1
	r.messages = append(r.messages, message)
	r.idempotentMessage = message
	return nil
}

func (r *messageAddSwitchRepo) CreateMessageWithPlanCommentsWithInitialTaskBrief(
	ctx context.Context,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
) (*models.TaskPlanCommentSnapshot, error) {
	r.messagesMu.Lock()
	if len(r.messages) == 0 {
		message.Content = candidate.Content
		candidate.Selected = true
	} else {
		candidate.Selected = false
	}
	r.messagesMu.Unlock()
	return r.CreateMessageWithPlanComments(ctx, message, refs, requirePrimary, expectedState, claim)
}

func (r *messageAddSwitchRepo) CreateMessageWithPlanCommentsAndQueueWithInitialTaskBrief(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	candidate *admission.InitialTaskBriefCandidate,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, error) {
	snapshot, err := r.CreateMessageWithPlanCommentsWithInitialTaskBrief(
		ctx, message, candidate, refs, requirePrimary, expectedState, claim,
	)
	if err != nil {
		return nil, err
	}
	queued.Content = message.Content
	copy := *queued
	r.queuedMessage = &copy
	_ = maxPerSession
	return snapshot, nil
}

type initialTaskBriefPromptOrchestrator struct {
	firstTurnCaptureOrchestrator
	expansions    map[string]string
	prepareInputs []string
}

func (o *initialTaskBriefPromptOrchestrator) PrepareDirectPrompt(
	_ context.Context,
	prompt string,
	_ bool,
) (string, string) {
	o.prepareInputs = append(o.prepareInputs, prompt)
	trustedContext := initialTaskBriefTrustedContext(prompt, o.expansions)
	if trustedContext == "" {
		return prompt, ""
	}
	return prompt + "\n\n" + sysprompt.Wrap(trustedContext), trustedContext
}

func initialTaskBriefTrustedContext(prompt string, expansions map[string]string) string {
	type reference struct {
		name  string
		index int
	}
	refs := make([]reference, 0, len(expansions))
	for name := range expansions {
		if index := strings.Index(prompt, "@"+name); index >= 0 {
			refs = append(refs, reference{name: name, index: index})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].index < refs[j].index })
	if len(refs) == 0 {
		return ""
	}
	lines := []string{"EXPANDED PROMPT REFERENCES:"}
	for _, ref := range refs {
		lines = append(lines, fmt.Sprintf("### @%s", ref.name), expansions[ref.name])
	}
	return strings.Join(lines, "\n")
}

func (o *firstTurnCaptureOrchestrator) StartCreatedSessionWithPromptContextAndCanvasGuidancePreservingDirectPrompt(
	_ context.Context,
	_, _, _, content string,
	_, _, _ bool,
	_ []v1.MessageAttachment,
	references []v1.EntityReference,
	promptReferenceContext string,
	_, _ bool,
) (*executor.TaskExecution, error) {
	o.started <- capturedFirstTurn{
		content:                content,
		references:             append([]v1.EntityReference(nil), references...),
		promptReferenceContext: promptReferenceContext,
	}
	return &executor.TaskExecution{}, nil
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.1, AC-TASKS-INITIAL-TASK-BRIEF-001.2
func TestWSAddMessage_InitialTaskBrief(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			taskID      = "task-initial-brief"
			sessionID   = "session-initial-brief"
			brief       = "Ship the authenticated task view."
			instruction = "Use the existing session and keep the change small."
		)
		now := time.Now().UTC()
		repo := &messageAddSwitchRepo{
			tasks: map[string]*models.Task{
				taskID: {
					ID:          taskID,
					Description: brief,
					State:       v1.TaskStateInProgress,
					UpdatedAt:   now,
				},
			},
			sessions: map[string]*models.TaskSession{
				sessionID: {
					ID:             sessionID,
					TaskID:         taskID,
					State:          models.TaskSessionStateCreated,
					AgentProfileID: "profile-initial-brief",
					UpdatedAt:      now,
				},
			},
			primaryID: sessionID,
		}
		log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		require.NoError(t, err)
		svc := service.NewService(service.Repos{
			Workspaces: repo, Tasks: repo, TaskRepos: repo,
			Workflows: repo, Messages: repo, Turns: repo,
			Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
			Executors: repo, Environments: repo, TaskEnvironments: repo,
			Reviews: repo,
		}, nil, log, service.RepositoryDiscoveryConfig{})
		orch := &firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)}
		h := NewMessageHandlers(svc, orch, log)

		req, err := ws.NewRequest("initial-brief-request", ws.ActionMessageAdd, map[string]interface{}{
			"task_id": taskID, "session_id": sessionID,
			"message_id": "initial-brief-message", "content": instruction,
		})
		require.NoError(t, err)

		resp, err := h.wsAddMessage(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		require.Len(t, repo.messages, 1)

		stored := repo.firstMessageContent()
		require.Contains(t, stored, brief)
		require.Contains(t, stored, instruction)
		require.Equal(t, 1, strings.Count(stored, brief))
		require.Equal(t, 1, strings.Count(stored, instruction))

		synctest.Wait()
		dispatched := <-orch.started
		require.Contains(t, dispatched.content, brief)
		require.Contains(t, dispatched.content, instruction)
		require.Equal(t, stored, dispatched.content)
	})
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.4, AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestWSAddMessage_InitialTaskBriefExpandsCombinedPromptAtAdmission(t *testing.T) {
	tests := []struct {
		name        string
		brief       string
		instruction string
		expansions  map[string]string
		wantVisible []string
		wantTrusted []string
		wantContext string
	}{
		{
			name:        "distinct references in brief and instruction",
			brief:       "Implement @brief_rules",
			instruction: "First follow @prep_rules",
			expansions: map[string]string{
				"brief_rules": "brief rules v1",
				"prep_rules":  "prep rules v1",
			},
			wantVisible: []string{"Implement @brief_rules", "First follow @prep_rules"},
			wantTrusted: []string{"brief rules v1", "prep rules v1"},
			wantContext: "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nbrief rules v1\n### @prep_rules\nprep rules v1",
		},
		{
			name:        "reference only in brief",
			brief:       "Implement @brief_rules",
			instruction: "Keep the existing session",
			expansions:  map[string]string{"brief_rules": "brief rules v1"},
			wantVisible: []string{"Implement @brief_rules", "Keep the existing session"},
			wantTrusted: []string{"brief rules v1"},
			wantContext: "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nbrief rules v1",
		},
		{
			name:        "identical brief and instruction",
			brief:       "Follow @rules",
			instruction: "Follow @rules",
			expansions:  map[string]string{"rules": "shared rules v1"},
			wantVisible: []string{"Follow @rules"},
			wantTrusted: []string{"shared rules v1"},
			wantContext: "EXPANDED PROMPT REFERENCES:\n### @rules\nshared rules v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const (
					taskID    = "task-initial-brief-prepared"
					sessionID = "session-initial-brief-prepared"
				)
				now := time.Now().UTC()
				repo := &messageAddSwitchRepo{
					tasks: map[string]*models.Task{
						taskID: {
							ID: taskID, Description: tt.brief, State: v1.TaskStateInProgress,
							UpdatedAt: now,
						},
					},
					sessions: map[string]*models.TaskSession{
						sessionID: {
							ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCreated,
							AgentProfileID: "profile-initial-brief-prepared", UpdatedAt: now,
						},
					},
					primaryID: sessionID,
				}
				log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
				require.NoError(t, err)
				orch := &initialTaskBriefPromptOrchestrator{
					firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{
						started: make(chan capturedFirstTurn, 1),
					},
					expansions: tt.expansions,
				}
				svc := service.NewService(service.Repos{
					Workspaces: repo, Tasks: repo, TaskRepos: repo,
					Workflows: repo, Messages: repo, Turns: repo,
					Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
					Executors: repo, Environments: repo, TaskEnvironments: repo,
					Reviews: repo,
				}, nil, log, service.RepositoryDiscoveryConfig{})
				h := NewMessageHandlers(svc, orch, log)

				req, err := ws.NewRequest("initial-brief-prepared-request", ws.ActionMessageAdd, map[string]interface{}{
					"task_id": taskID, "session_id": sessionID,
					"message_id": "initial-brief-prepared-message", "content": tt.instruction,
				})
				require.NoError(t, err)

				resp, err := h.wsAddMessage(context.Background(), req)
				require.NoError(t, err)
				require.Equal(t, ws.MessageTypeResponse, resp.Type)
				require.Len(t, repo.messages, 1)
				require.Len(t, orch.prepareInputs, 2)
				require.Equal(t, tt.instruction, orch.prepareInputs[0])
				require.Equal(t, composeInitialTaskBrief(tt.brief, tt.instruction), orch.prepareInputs[1])

				stored := repo.firstMessageContent()
				for _, visible := range tt.wantVisible {
					require.Equal(t, 1, strings.Count(stored, visible), "visible prompt %q in %q", visible, stored)
				}
				for _, trusted := range tt.wantTrusted {
					require.Equal(t, 1, strings.Count(stored, trusted), "trusted expansion %q in %q", trusted, stored)
				}
				require.Contains(t, stored, sysprompt.Wrap(tt.wantContext))

				synctest.Wait()
				dispatched := <-orch.started
				require.Equal(t, stored, dispatched.content)
				require.Equal(t, tt.wantContext, dispatched.promptReferenceContext)
			})
		})
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestWSAddMessage_InitialTaskBriefKeepsAcceptedExpansionWhenDefinitionsChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			taskID      = "task-initial-brief-snapshot"
			sessionID   = "session-initial-brief-snapshot"
			brief       = "Implement @brief_rules"
			instruction = "First follow @prep_rules"
		)
		now := time.Now().UTC()
		repo := &messageAddSwitchRepo{
			tasks: map[string]*models.Task{taskID: {
				ID: taskID, Description: brief, State: v1.TaskStateInProgress, UpdatedAt: now,
			}},
			sessions: map[string]*models.TaskSession{sessionID: {
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCreated,
				AgentProfileID: "profile-initial-brief-snapshot", UpdatedAt: now,
			}},
			primaryID: sessionID,
		}
		log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		require.NoError(t, err)
		orch := &initialTaskBriefPromptOrchestrator{
			firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)},
			expansions: map[string]string{
				"brief_rules": "brief rules accepted",
				"prep_rules":  "prep rules accepted",
			},
		}
		svc := service.NewService(service.Repos{
			Workspaces: repo, Tasks: repo, TaskRepos: repo,
			Workflows: repo, Messages: repo, Turns: repo,
			Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
			Executors: repo, Environments: repo, TaskEnvironments: repo,
			Reviews: repo,
		}, nil, log, service.RepositoryDiscoveryConfig{})
		h := NewMessageHandlers(svc, orch, log)
		req, err := ws.NewRequest("initial-brief-snapshot-request", ws.ActionMessageAdd, map[string]interface{}{
			"task_id": taskID, "session_id": sessionID,
			"message_id": "initial-brief-snapshot-message", "content": instruction,
		})
		require.NoError(t, err)

		resp, err := h.wsAddMessage(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		orch.expansions["brief_rules"] = "brief rules changed before dispatch"
		orch.expansions["prep_rules"] = "prep rules changed before dispatch"

		stored := repo.firstMessageContent()
		expectedContext := "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nbrief rules accepted\n### @prep_rules\nprep rules accepted"
		require.Contains(t, stored, sysprompt.Wrap(expectedContext))
		require.NotContains(t, stored, "changed before dispatch")
		synctest.Wait()
		dispatched := <-orch.started
		require.Equal(t, expectedContext, dispatched.promptReferenceContext)
	})
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestWSAddMessage_QueuedInitialTaskBriefPersistsAcceptedExpansion(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-initial-brief-queued": {
			ID: "task-initial-brief-queued", Description: "Implement @brief_rules",
			State: v1.TaskStateInProgress, UpdatedAt: now,
		}},
		sessions: map[string]*models.TaskSession{"session-initial-brief-queued": {
			ID: "session-initial-brief-queued", TaskID: "task-initial-brief-queued",
			State: models.TaskSessionStateCreated, AgentProfileID: "profile-initial-brief-queued", UpdatedAt: now,
		}},
		primaryID: "session-initial-brief-queued",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	orch := &initialTaskBriefPromptOrchestrator{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)},
		expansions:                   map[string]string{"brief_rules": "queued brief rules accepted"},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := NewMessageHandlers(svc, orch, log)
	req, err := ws.NewRequest("initial-brief-queued-request", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "task-initial-brief-queued", "session_id": "session-initial-brief-queued",
		"message_id": "initial-brief-queued-message", "content": "Keep the existing session",
		"plan_comment_refs":       []map[string]interface{}{{"id": "comment-handler", "version": 3}},
		"require_primary_session": true,
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, repo.messages, 1)
	require.NotNil(t, repo.queuedMessage)
	require.Equal(t, repo.messages[0].Content, repo.queuedMessage.Content)
	expectedContext := "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nqueued brief rules accepted"
	require.Contains(t, repo.queuedMessage.Content, sysprompt.Wrap(expectedContext))
	require.NotContains(t, repo.queuedMessage.Content, "\x00")
	require.Empty(t, orch.started)
}
