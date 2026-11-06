package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	utility "github.com/kandev/kandev/internal/agentctl/server/utility"
)

type promptRouteResolver struct {
	failures int
	resumes  int
	failure  func(string, int64, *routingerr.Error) (agentruntime.ProfileExecution, error)
	resume   func(context.Context, string, int64) (agentruntime.ProfileExecution, error)
}

func (r *promptRouteResolver) ResolveExecutionAfterFailure(_ context.Context, session, logical, current string, generation int64, failure *routingerr.Error) (agentruntime.ProfileExecution, error) {
	r.failures++
	return r.failure(current, generation, failure)
}
func (r *promptRouteResolver) ResumePendingRoute(ctx context.Context, session string, generation int64) (agentruntime.ProfileExecution, error) {
	r.resumes++
	return r.resume(ctx, session, generation)
}
func promptTestRoute() *PromptRequest {
	return &PromptRequest{AgentProfileID: "logical", ExecutionProfileID: "a", RouteSessionID: "isolated-route", RouteGeneration: 7, AgentCLI: "claude-acp", Model: "model-a"}
}
func promptTestSuccessor(id string, generation int64) agentruntime.ProfileExecution {
	return agentruntime.ProfileExecution{ExecutionProfileID: id, RouteSessionID: "isolated-route", Generation: generation, AgentName: "codex-acp", Profile: &settings.AgentProfile{ID: id, AgentID: "agent-row", Model: "model-b"}}
}
func TestProfilePromptFallbackRetainsIdentity(t *testing.T) {
	route := promptTestRoute()
	origin := agentruntime.InferenceIdentity{TaskID: "task", SessionID: "session", LogicalProfileID: "logical", CallID: "call", ReviewRunID: "review", OwnerKind: "utility"}
	ctx := agentruntime.WithInferenceIdentity(context.Background(), origin)
	resolver := &promptRouteResolver{failure: func(id string, generation int64, failure *routingerr.Error) (agentruntime.ProfileExecution, error) {
		if id != "a" || generation != 7 || !failure.FallbackAllowed {
			t.Fatalf("wrong failure attribution: %s/%d %+v", id, generation, failure)
		}
		return promptTestSuccessor("b", 8), nil
	}}
	var calls []string
	response, err := ExecuteProfilePrompt(ctx, resolver, route, func(attempt context.Context, id string) (*utility.PromptResponse, error) {
		calls = append(calls, id)
		expected := origin
		expected.ExecutionProfileID = id
		if got := agentruntime.InferenceIdentityFromContext(attempt); got != expected {
			t.Fatalf("origin changed: %+v", got)
		}
		if id == "a" {
			return nil, errors.New("rate limit exceeded")
		}
		return &utility.PromptResponse{Success: true, Response: "answer"}, nil
	})
	if err != nil || response == nil || response.Response != "answer" || !reflect.DeepEqual(calls, []string{"a", "b"}) {
		t.Fatalf("calls=%v response=%+v err=%v", calls, response, err)
	}
	if route.ExecutionProfileID != "b" || route.RouteGeneration != 8 || route.AgentCLI != "codex-acp" || route.Model != "model-b" || resolver.failures != 1 {
		t.Fatalf("route=%+v resolver=%+v", route, resolver)
	}
}
func TestProfilePromptStopsWithoutSuccessor(t *testing.T) {
	for _, kind := range []string{"deadline", "wrapped deadline", "partial and error", "capacity", "parent before", "parent during"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := errors.New("rate limit exceeded")
			var response *utility.PromptResponse
			switch kind {
			case "deadline":
				cause = context.DeadlineExceeded
			case "wrapped deadline":
				cause = fmt.Errorf("rate limit exceeded: %w", context.DeadlineExceeded)
			case "partial and error":
				response = &utility.PromptResponse{Response: "visible partial"}
			case "capacity":
				cause = &agentruntime.InferenceCapacityError{Reason: "busy", Cause: context.DeadlineExceeded}
			case "parent before":
				cancel()
				cause = context.Canceled
			case "parent during":
				cause = context.Canceled
			}
			calls := 0
			resolver := &promptRouteResolver{failure: func(string, int64, *routingerr.Error) (agentruntime.ProfileExecution, error) {
				t.Fatal("stopped result reached failure policy")
				return agentruntime.ProfileExecution{}, nil
			}}
			got, err := ExecuteProfilePrompt(ctx, resolver, promptTestRoute(), func(context.Context, string) (*utility.PromptResponse, error) {
				calls++
				if kind == "parent during" {
					cancel()
				}
				return response, cause
			})
			wantCalls := 1
			if kind == "parent before" {
				wantCalls = 0
			}
			if calls != wantCalls || resolver.failures != 0 || got != response || !errors.Is(err, cause) {
				t.Fatalf("calls=%d failure reports=%d response=%+v err=%v cause=%v", calls, resolver.failures, got, err, cause)
			}
		})
	}
}
func TestProfilePromptEightFailuresNoNinthDispatchOrWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		calls := 0
		cause := errors.New("rate limit exceeded")
		resolver := &promptRouteResolver{}
		resolver.failure = func(current string, generation int64, failure *routingerr.Error) (agentruntime.ProfileExecution, error) {
			if current != "a" || generation != 7 {
				t.Fatalf("wrong attribution %s/%d", current, generation)
			}
			next := promptTestSuccessor("a", 7)
			next.AgentName = "claude-acp"
			if resolver.failures == 8 {
				deadline := time.Now().Add(time.Hour)
				next.Decision.Deadline = &deadline
				return next, dynamic.ErrRecoveryPending
			}
			return next, nil
		}
		resolver.resume = func(context.Context, string, int64) (agentruntime.ProfileExecution, error) {
			t.Fatal("eighth failure waited")
			return agentruntime.ProfileExecution{}, nil
		}
		_, err := ExecuteProfilePrompt(context.Background(), resolver, promptTestRoute(), func(context.Context, string) (*utility.PromptResponse, error) { calls++; return nil, cause })
		if calls != 8 || resolver.failures != 8 || resolver.resumes != 0 || !time.Now().Equal(start) || !errors.Is(err, cause) || !errors.Is(err, dynamic.ErrRecoveryPending) {
			t.Fatalf("calls=%d reports=%d resumes=%d elapsed=%s err=%v", calls, resolver.failures, resolver.resumes, time.Since(start), err)
		}
	})
}
func TestProfilePromptPendingTimer(t *testing.T) {
	for _, kind := range []string{"due", "nil deadline", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				start := time.Now()
				deadline := start.Add(time.Second)
				next := promptTestSuccessor("a", 7)
				if kind != "nil deadline" {
					next.Decision.Deadline = &deadline
				}
				resolver := &promptRouteResolver{failure: func(string, int64, *routingerr.Error) (agentruntime.ProfileExecution, error) {
					return next, dynamic.ErrRecoveryPending
				}}
				resolver.resume = func(ctx context.Context, session string, generation int64) (agentruntime.ProfileExecution, error) {
					if ctx.Err() != nil || session != "isolated-route" || generation != 7 || time.Now().Before(deadline) {
						t.Fatalf("premature/unowned resume: %s/%d %v", session, generation, ctx.Err())
					}
					return promptTestSuccessor("b", 8), nil
				}
				if kind == "cancel" {
					go func() { time.Sleep(time.Millisecond); cancel() }()
				}
				calls := 0
				_, err := ExecuteProfilePrompt(ctx, resolver, promptTestRoute(), func(_ context.Context, id string) (*utility.PromptResponse, error) {
					calls++
					if id == "a" {
						return nil, errors.New("rate limit exceeded")
					}
					return &utility.PromptResponse{Success: true}, nil
				})
				switch kind {
				case "due":
					if err != nil || calls != 2 || resolver.resumes != 1 || time.Since(start) != time.Second {
						t.Fatalf("calls=%d resumes=%d elapsed=%s err=%v", calls, resolver.resumes, time.Since(start), err)
					}
				case "nil deadline":
					if !errors.Is(err, dynamic.ErrRecoveryPending) || calls != 1 || resolver.resumes != 0 || !time.Now().Equal(start) {
						t.Fatalf("nil deadline: calls=%d resumes=%d err=%v", calls, resolver.resumes, err)
					}
				case "cancel":
					if !errors.Is(err, context.Canceled) || calls != 1 || resolver.resumes != 0 {
						t.Fatalf("cancel: calls=%d resumes=%d err=%v", calls, resolver.resumes, err)
					}
				}
			})
		})
	}
}
