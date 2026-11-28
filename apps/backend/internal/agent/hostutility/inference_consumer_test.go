package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	utility "github.com/kandev/kandev/internal/agentctl/server/utility"
)

func TestInferenceConsumerHostActualEntriesPrepareOneToken(t *testing.T) {
	for _, profile := range []bool{false, true} {
		for _, mode := range []string{"owned", "allowed", "blocked"} {
			t.Run(map[bool]string{false: "raw", true: "profile"}[profile]+"_"+mode, func(t *testing.T) {
				log := newTestLogger(t)
				reg := registry.NewRegistry(log)
				const agentType = "codex-acp"
				if err := reg.Register(&installedInferenceAgent{id: agentType}); err != nil {
					t.Fatal(err)
				}
				var prepared, prompts atomic.Int32
				attempt := uuid.NewString()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/health":
						w.WriteHeader(200)
					case "/api/v1/inference/attempts":
						prepared.Add(1)
						var request utility.PrepareAttemptRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						_ = json.NewEncoder(w).Encode(utility.AttemptStatus{PreparationID: request.PreparationID, AttemptID: attempt, InstanceID: "instance", OwnerID: "owner", Known: true, State: utility.AttemptPrepared})
					case "/api/v1/inference/prompt":
						prompts.Add(1)
						var request utility.PromptRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						if request.AttemptID != attempt {
							t.Errorf("actual host prompt lacks issued token: %+v", request)
						}
						_ = json.NewEncoder(w).Encode(utility.PromptResponse{Success: true, Response: "fixture", Model: request.Model})
					default:
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				host, port := serverHostPort(t, server)
				manager := NewManager(reg, host, port, nil, log)
				manager.parentTmpDir = t.TempDir()
				client := agentctlclient.NewClient(host, port, log, agentctlclient.WithExecutionID("instance"))
				defer client.Close()
				manager.instances[agentType] = &instance{agentType: agentType, instanceID: "instance", workDir: filepath.Join(manager.parentTmpDir, agentType), client: client}
				manager.SetProfileResolver(profilePromptResolver{profile: &settingsmodels.AgentProfile{ID: "profile", AgentID: agentType, Model: "explicit-model"}})
				original := agentctlclient.InferenceIdentity{LogicalProfileID: "logical-original", ExecutionProfileID: "physical-original", TaskID: "task", SessionID: "session", CallID: uuid.NewString(), ReviewRunID: uuid.NewString()}
				if mode == "foreign_task" {
					original.TaskID = "other-task"
				}
				if mode == "foreign_session" {
					original.SessionID = "other-session"
				}
				ctx := agentctlclient.WithInferenceIdentity(context.Background(), original)
				calls := 0
				if mode != "owned" {
					manager.SetInferenceConsumer(OwnedConsumerFunc(func(ctx context.Context, client *agentctlclient.Client, request *utility.PromptRequest, identity agentctlclient.InferenceIdentity) (*utility.PromptResponse, error) {
						calls++
						wantProfile := "physical-original"
						if profile {
							wantProfile = "profile"
						}
						if identity.LogicalProfileID != "logical-original" || identity.ExecutionProfileID != wantProfile || identity.CallID != original.CallID || identity.ReviewRunID != original.ReviewRunID || request.Model != "explicit-model" {
							t.Errorf("logical/physical request identity changed: %+v / %+v", identity, request)
						}
						if identity.OwnerKind != "host" || identity.ExecutorType != "standalone" || identity.TaskID != "task" || identity.SessionID != "session" {
							t.Errorf("host relationships changed: %+v", identity)
						}
						if mode == "blocked" {
							return nil, &agentctlclient.InferenceCapacityError{Reason: "fixture_capacity_wait"}
						}
						return client.InferencePromptOwned(ctx, request)
					}))
				}
				var result *PromptResult
				var err error
				if profile {
					result, err = manager.ExecuteProfilePrompt(ctx, "profile", "fixture")
				} else {
					result, err = manager.ExecutePrompt(ctx, agentType, "explicit-model", "", "fixture")
				}
				if mode == "blocked" || mode == "foreign_task" || mode == "foreign_session" {
					wantCalls := 1
					if mode != "blocked" {
						wantCalls = 0
					}
					if !agentctlclient.IsInferenceCapacityError(err) || result != nil || calls != wantCalls || prompts.Load() != 0 {
						t.Fatalf("refusal/foreign context escaped: result=%+v err=%v consumer=%d prompts=%d", result, err, calls, prompts.Load())
					}
					return
				}
				if mode == "allowed" && calls != 1 {
					t.Fatalf("shared consumer calls=%d", calls)
				}
				if err != nil || result == nil || result.Model != "explicit-model" || prepared.Load() != 1 || prompts.Load() != 1 {
					t.Fatalf("actual entry bypassed ownership: result=%+v err=%v prepare=%d prompts=%d", result, err, prepared.Load(), prompts.Load())
				}
			})
		}
	}
}

type OwnedConsumerFunc func(context.Context, *agentctlclient.Client, *utility.PromptRequest, agentctlclient.InferenceIdentity) (*utility.PromptResponse, error)

func (f OwnedConsumerFunc) Execute(ctx context.Context, client *agentctlclient.Client, request *utility.PromptRequest, identity agentctlclient.InferenceIdentity) (*utility.PromptResponse, error) {
	return f(ctx, client, request, identity)
}

func TestInferenceConsumerHostCacheDoesNotProveCompleteInventory(t *testing.T) {
	refs, err := (&Manager{}).InferenceClients(context.Background())
	for _, ref := range refs {
		ref.Release()
	}
	if err == nil {
		t.Fatal("unbound local cache was accepted as a complete empty census")
	}
}
