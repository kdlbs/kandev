package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	utility "github.com/kandev/kandev/internal/agentctl/server/utility"
)

func TestInferenceConsumerSessionActualEntriesPrepareOneToken(t *testing.T) {
	for _, profile := range []bool{false, true} {
		for _, mode := range []string{"owned", "allowed", "blocked", "foreign_task", "foreign_session"} {
			t.Run(map[bool]string{false: "raw", true: "profile"}[profile]+"_"+mode, func(t *testing.T) {
				var prepared, prompts atomic.Int32
				attempt := uuid.NewString()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v1/inference/attempts":
						prepared.Add(1)
						var request utility.PrepareAttemptRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						_ = json.NewEncoder(w).Encode(utility.AttemptStatus{PreparationID: request.PreparationID, AttemptID: attempt, InstanceID: "parent-instance", OwnerID: "owner", Known: true, State: utility.AttemptPrepared})
					case "/api/v1/inference/prompt":
						prompts.Add(1)
						var request utility.PromptRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						if request.AttemptID != attempt {
							t.Errorf("actual session prompt lacks issued token: %+v", request)
						}
						_ = json.NewEncoder(w).Encode(utility.PromptResponse{Success: true, Response: "fixture", Model: request.Model})
					default:
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				parsed, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				port, err := strconv.Atoi(parsed.Port())
				if err != nil {
					t.Fatal(err)
				}
				reg := newTestRegistry()
				if err := reg.Register(newInferenceAgent("inference-ok", true, true)); err != nil {
					t.Fatal(err)
				}
				resolver := &stubProfileResolver{profile: &AgentProfileInfo{AgentID: "inference-ok", Model: "explicit-model"}}
				manager := NewManager(reg, &MockEventBus{}, nil, &MockCredentialsManager{}, resolver, nil, ExecutorFallbackWarn, "", newTestLogger())
				cleanupManagerStopCh(t, manager)
				client := agentctl.NewClient(parsed.Hostname(), port, newTestLogger(), agentctl.WithExecutionID("parent-instance"))
				defer client.Close()
				if err := manager.executionStore.Add(&AgentExecution{ID: "parent-instance", TaskID: "task", SessionID: "session", WorkspacePath: t.TempDir(), agentctl: client}); err != nil {
					t.Fatal(err)
				}
				original := agentctl.InferenceIdentity{LogicalProfileID: "logical-original", ExecutionProfileID: "physical-original", TaskID: "task", SessionID: "session", CallID: uuid.NewString(), ReviewRunID: uuid.NewString()}
				if mode == "foreign_task" {
					original.TaskID = "other-task"
				}
				if mode == "foreign_session" {
					original.SessionID = "other-session"
				}
				ctx := agentctl.WithInferenceIdentity(context.Background(), original)
				calls := 0
				if mode != "owned" {
					manager.SetInferenceConsumer(OwnedConsumerFunc(func(ctx context.Context, client *agentctl.Client, request *utility.PromptRequest, identity agentctl.InferenceIdentity) (*utility.PromptResponse, error) {
						calls++
						wantProfile := "physical-original"
						if profile {
							wantProfile = "profile"
						}
						if identity.LogicalProfileID != "logical-original" || identity.ExecutionProfileID != wantProfile || identity.CallID != original.CallID || identity.ReviewRunID != original.ReviewRunID || request.Model != "explicit-model" {
							t.Errorf("logical/physical request identity changed: %+v / %+v", identity, request)
						}
						if mode == "allowed" && (identity.OwnerKind != "session" || identity.ParentExecutionID != "parent-instance" || identity.TaskID != "task" || identity.SessionID != "session") {
							t.Errorf("execution relationships missing: %+v", identity)
						}
						if mode == "blocked" {
							return nil, &agentctl.InferenceCapacityError{Reason: "fixture_capacity_wait"}
						}
						return client.InferencePromptOwned(ctx, request)
					}))
				}
				var result *utility.PromptResponse
				if profile {
					result, err = manager.ExecuteInferenceProfilePrompt(ctx, "session", "profile", "fixture")
				} else {
					result, err = manager.ExecuteInferencePrompt(ctx, "session", "inference-ok", "explicit-model", "fixture")
				}
				if mode == "blocked" || mode == "foreign_task" || mode == "foreign_session" {
					wantCalls := 1
					if mode != "blocked" {
						wantCalls = 0
					}
					if !agentctl.IsInferenceCapacityError(err) || result != nil || calls != wantCalls || prompts.Load() != 0 {
						t.Fatalf("refusal/foreign context escaped: result=%+v err=%v consumer=%d prompts=%d", result, err, calls, prompts.Load())
					}
					return
				}
				if mode == "allowed" && calls != 1 {
					t.Fatalf("shared consumer calls=%d", calls)
				}
				if err != nil || result == nil || result.Model != "explicit-model" || prepared.Load() != 1 || prompts.Load() != 1 {
					t.Fatalf("actual session entry bypassed ownership: result=%+v err=%v prepare=%d prompts=%d", result, err, prepared.Load(), prompts.Load())
				}
			})
		}
	}
}

type OwnedConsumerFunc func(context.Context, *agentctl.Client, *utility.PromptRequest, agentctl.InferenceIdentity) (*utility.PromptResponse, error)

func (f OwnedConsumerFunc) Execute(ctx context.Context, client *agentctl.Client, request *utility.PromptRequest, identity agentctl.InferenceIdentity) (*utility.PromptResponse, error) {
	return f(ctx, client, request, identity)
}
