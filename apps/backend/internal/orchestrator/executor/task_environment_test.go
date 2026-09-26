package executor

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/task/models"
)

type environmentSourceResolver func(context.Context, environmentSource) (string, error)

// resolveEnvironmentSources is test-only: it exists to exercise
// runtimeenv.Resolve against the environmentSource shape without a
// task/session context to log or count overrides against. It discards the
// OverrideRecords Resolve returns and SHALL NOT log or count on its own.
func resolveEnvironmentSources(
	ctx context.Context, sources []environmentSource, resolve environmentSourceResolver,
) (map[string]string, error) {
	definitions := make([]runtimeenv.Definition, 0, len(sources))
	for _, source := range sources {
		definitions = append(definitions, runtimeenv.Definition{
			Key: source.key, Literal: source.literal, SecretID: source.secretID,
			Origin: source.origin, WorkspaceID: source.workspaceID,
		})
	}
	resolved, _, err := runtimeenv.Resolve(ctx, definitions, func(ctx context.Context, definition runtimeenv.Definition) (string, error) {
		return resolve(ctx, environmentSource{
			key: definition.Key, literal: definition.Literal, secretID: definition.SecretID,
			origin: definition.Origin, workspaceID: definition.WorkspaceID,
		})
	})
	return resolved, err
}

func TestResolveEnvironmentSources_DeduplicatesSameSecretAndIgnoresOrder(t *testing.T) {
	resolver := func(_ context.Context, source environmentSource) (string, error) {
		return map[string]string{"secret-a": "alpha", "secret-b": "bravo"}[source.secretID], nil
	}
	first := []environmentSource{
		{key: "TOKEN", secretID: "secret-a", origin: "repository app"},
		{key: "TOKEN", secretID: "secret-a", origin: "executor profile"},
		{key: "OTHER", secretID: "secret-b", origin: "repository tools"},
	}
	second := []environmentSource{first[2], first[0], first[1]}

	gotFirst, err := resolveEnvironmentSources(context.Background(), first, resolver)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	gotSecond, err := resolveEnvironmentSources(context.Background(), second, resolver)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if len(gotFirst) != 2 || gotFirst["TOKEN"] != "alpha" || gotFirst["OTHER"] != "bravo" {
		t.Fatalf("first result = %#v", gotFirst)
	}
	if gotSecond["TOKEN"] != gotFirst["TOKEN"] || gotSecond["OTHER"] != gotFirst["OTHER"] {
		t.Fatalf("order changed result: first=%#v second=%#v", gotFirst, gotSecond)
	}
}

func TestResolveEnvironmentSources_RejectsEveryConflictingPair(t *testing.T) {
	cases := []struct {
		name  string
		first environmentSource
		last  environmentSource
	}{
		{name: "managed and executor literal", first: environmentSource{key: "TOKEN", literal: "managed", origin: "managed/runtime"}, last: environmentSource{key: "TOKEN", literal: "profile", origin: "executor profile"}},
		{name: "managed and repository secret", first: environmentSource{key: "TOKEN", literal: "managed", origin: "managed/runtime"}, last: environmentSource{key: "TOKEN", secretID: "secret-a", origin: "repository app"}},
		{name: "executor and repository secrets", first: environmentSource{key: "TOKEN", secretID: "secret-a", origin: "executor profile"}, last: environmentSource{key: "TOKEN", secretID: "secret-b", origin: "repository app"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveEnvironmentSources(context.Background(), []environmentSource{tc.first, tc.last}, func(context.Context, environmentSource) (string, error) {
				return "not-used", nil
			})
			if err == nil {
				t.Fatal("resolve succeeded, want conflict")
			}
			var conflictErr *EnvironmentConflictError
			if !errors.As(err, &conflictErr) {
				t.Fatalf("error = %T %v, want EnvironmentConflictError", err, err)
			}
			for _, leaked := range []string{"secret-a", "secret-b", "not-used"} {
				if strings.Contains(err.Error(), leaked) {
					t.Fatalf("error %q leaks %q", err, leaked)
				}
			}
		})
	}
}

func TestResolveEnvironmentSources_ReportsEveryConflictingOrigin(t *testing.T) {
	_, err := resolveEnvironmentSources(context.Background(), []environmentSource{
		{key: "TOKEN", secretID: "secret-a", origin: "repository app"},
		{key: "TOKEN", secretID: "secret-b", origin: "repository tools"},
		{key: "TOKEN", literal: "profile-value", origin: "executor profile"},
		{key: "OTHER", literal: "unrelated", origin: "repository docs"},
	}, func(context.Context, environmentSource) (string, error) {
		return "unused", nil
	})
	if err == nil {
		t.Fatal("resolve succeeded, want conflict")
	}
	var conflictErr *EnvironmentConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("error = %T %v, want EnvironmentConflictError", err, err)
	}
	want := []string{"executor profile", "repository app", "repository tools"}
	if !reflect.DeepEqual(conflictErr.Origins, want) {
		t.Fatalf("conflict origins = %#v, want %#v", conflictErr.Origins, want)
	}
}

func TestResolveLaunchEnvironment_PreferredShellWinsOverProfileShell(t *testing.T) {
	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{
		WorkspaceID:  "workspace-1",
		ExecutorType: string(models.ExecutorTypeLocal),
		Env:          map[string]string{},
	}

	err := executor.resolveLaunchEnvironment(context.Background(), req, []models.ProfileEnvVar{
		{Key: "SHELL", Value: "/bin/zsh"},
		{Key: "AGENTCTL_SHELL_COMMAND", Value: "/bin/fish"},
		{Key: "PROFILE_TOKEN", Value: "profile-value"},
	}, nil)
	if err != nil {
		t.Fatalf("resolve launch environment: %v", err)
	}
	if req.Env["SHELL"] != "/bin/bash" || req.Env["AGENTCTL_SHELL_COMMAND"] != "/bin/bash" {
		t.Fatalf("preferred shell environment = %#v, want bash values", req.Env)
	}
	if len(req.EnvironmentDefinitions) != 3 {
		t.Fatalf("environment definitions = %#v, want preferred shell and profile token", req.EnvironmentDefinitions)
	}
}

func TestResolveEnvironmentSources_RedactsFailedSecretReference(t *testing.T) {
	_, err := resolveEnvironmentSources(context.Background(), []environmentSource{{
		key: "TOKEN", secretID: "secret-private", origin: "repository app",
	}}, func(context.Context, environmentSource) (string, error) {
		return "", errors.New("secret-private backend failure")
	})
	if err == nil || !strings.Contains(err.Error(), "TOKEN") || !strings.Contains(err.Error(), "repository app") {
		t.Fatalf("error = %v, want key and origin", err)
	}
	if strings.Contains(err.Error(), "secret-private") || strings.Contains(err.Error(), "backend failure") {
		t.Fatalf("error %q leaks secret details", err)
	}
}
