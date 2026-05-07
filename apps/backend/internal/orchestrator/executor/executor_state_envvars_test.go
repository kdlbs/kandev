package executor

import (
	"context"
	"reflect"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func TestResolveAgentEnvVars_Empty(t *testing.T) {
	t.Parallel()
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	if got := exec.resolveAgentEnvVars(context.Background(), nil); got != nil {
		t.Errorf("expected nil for nil input, got %#v", got)
	}
	if got := exec.resolveAgentEnvVars(context.Background(), []settingsmodels.EnvVar{}); got != nil {
		t.Errorf("expected nil for empty input, got %#v", got)
	}
}

func TestResolveAgentEnvVars_PlainValues(t *testing.T) {
	t.Parallel()
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	in := []settingsmodels.EnvVar{
		{Key: "FOO", Value: "bar"},
		{Key: "BAZ", Value: "qux"},
	}
	got := exec.resolveAgentEnvVars(context.Background(), in)
	want := map[string]string{"FOO": "bar", "BAZ": "qux"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestResolveAgentEnvVars_SecretRef(t *testing.T) {
	t.Parallel()
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.secretStore = &mockSecretStore{
		secrets: map[string]string{"sec-1": "tok-abc", "sec-2": "tok-def"},
	}
	in := []settingsmodels.EnvVar{
		{Key: "JIRA_TOKEN", SecretID: "sec-1"},
		{Key: "CONFLUENCE_TOKEN", SecretID: "sec-2"},
		{Key: "PLAIN", Value: "literal"},
	}
	got := exec.resolveAgentEnvVars(context.Background(), in)
	want := map[string]string{
		"JIRA_TOKEN":       "tok-abc",
		"CONFLUENCE_TOKEN": "tok-def",
		"PLAIN":            "literal",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestResolveAgentEnvVars_MissingSecretSkipped(t *testing.T) {
	t.Parallel()
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.secretStore = &mockSecretStore{secrets: map[string]string{"sec-1": "ok"}}
	in := []settingsmodels.EnvVar{
		{Key: "GOOD", SecretID: "sec-1"},
		{Key: "MISSING", SecretID: "sec-deleted"},
		{Key: "PLAIN", Value: "p"},
	}
	got := exec.resolveAgentEnvVars(context.Background(), in)
	// MISSING is dropped silently (warning logged); other entries survive.
	if _, present := got["MISSING"]; present {
		t.Errorf("MISSING with deleted secret leaked into result: %#v", got)
	}
	if got["GOOD"] != "ok" || got["PLAIN"] != "p" {
		t.Errorf("expected good entries preserved, got %#v", got)
	}
}

func TestResolveAgentEnvVars_NilSecretStoreSkipsSecretRefs(t *testing.T) {
	t.Parallel()
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	// secretStore intentionally not set — defaults to nil.
	in := []settingsmodels.EnvVar{
		{Key: "FOO", SecretID: "sec-1"}, // skipped: no store
		{Key: "BAR", Value: "v"},
	}
	got := exec.resolveAgentEnvVars(context.Background(), in)
	if _, present := got["FOO"]; present {
		t.Errorf("secret-ref entry leaked when secretStore is nil: %#v", got)
	}
	if got["BAR"] != "v" {
		t.Errorf("expected BAR=v, got %#v", got)
	}
}
