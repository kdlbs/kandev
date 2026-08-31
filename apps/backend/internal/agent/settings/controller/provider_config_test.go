package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

// newProviderTestController wires a sqlite-backed controller with the default
// agent registry (so codex-acp advertises OpenAI-compatible provider support)
// and seeds one supporting agent row (codex-acp) plus one non-supporting row
// (claude-acp). It returns the two agent IDs.
func newProviderTestController(t *testing.T) (*Controller, store.Repository, context.Context, string, string) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, cleanup, err := store.Provide(db, db, log)
	if err != nil {
		t.Fatalf("settings store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	ctrl := NewController(repo, nil, reg, nil, log)

	ctx := context.Background()
	codex := &models.Agent{Name: "codex-acp"}
	if err := repo.CreateAgent(ctx, codex); err != nil {
		t.Fatalf("seed codex agent: %v", err)
	}
	claude := &models.Agent{Name: "claude-acp"}
	if err := repo.CreateAgent(ctx, claude); err != nil {
		t.Fatalf("seed claude agent: %v", err)
	}
	return ctrl, repo, ctx, codex.ID, claude.ID
}

func TestCreateProfile_OpenAICompatibleProvider_RoundTrips(t *testing.T) {
	ctrl, repo, ctx, codexID, _ := newProviderTestController(t)

	got, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID:                codexID,
		Name:                   "9router",
		Model:                  "code",
		ProviderKind:           models.ProviderKindOpenAICompatible,
		ProviderBaseURL:        "http://localhost:20128/v1",
		ProviderAPIKeySecretID: "",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ProviderKind != models.ProviderKindOpenAICompatible {
		t.Errorf("ProviderKind = %q", got.ProviderKind)
	}
	if got.ProviderBaseURL != "http://localhost:20128/v1" {
		t.Errorf("ProviderBaseURL = %q", got.ProviderBaseURL)
	}
	if !got.ProviderSupported {
		t.Errorf("ProviderSupported = false, want true for codex-acp")
	}

	reread, err := repo.GetAgentProfile(ctx, got.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reread.ProviderBaseURL != "http://localhost:20128/v1" || reread.ProviderKind != models.ProviderKindOpenAICompatible {
		t.Errorf("reread provider fields not persisted: %+v", reread)
	}
}

func TestCreateProfile_OpenAICompatibleProvider_Rejections(t *testing.T) {
	ctrl, _, ctx, codexID, claudeID := newProviderTestController(t)

	cases := []struct {
		name string
		req  CreateProfileRequest
	}{
		{"missing base URL", CreateProfileRequest{AgentID: codexID, Name: "a", Model: "code", ProviderKind: models.ProviderKindOpenAICompatible}},
		{"relative base URL", CreateProfileRequest{AgentID: codexID, Name: "a", Model: "code", ProviderKind: models.ProviderKindOpenAICompatible, ProviderBaseURL: "/v1"}},
		{"slash in model", CreateProfileRequest{AgentID: codexID, Name: "a", Model: "openai/gpt-4o", ProviderKind: models.ProviderKindOpenAICompatible, ProviderBaseURL: "http://localhost:20128/v1"}},
		{"agent unsupported", CreateProfileRequest{AgentID: claudeID, Name: "a", Model: "sonnet", ProviderKind: models.ProviderKindOpenAICompatible, ProviderBaseURL: "http://localhost:20128/v1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ctrl.CreateProfile(ctx, tc.req); !errors.Is(err, ErrInvalidProviderConfig) {
				t.Fatalf("err = %v, want ErrInvalidProviderConfig", err)
			}
		})
	}
}

func TestCreateProfile_NativeClearsProviderFields(t *testing.T) {
	ctrl, _, ctx, codexID, _ := newProviderTestController(t)

	got, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID:         codexID,
		Name:            "native",
		Model:           "code",
		ProviderKind:    models.ProviderKindNative,
		ProviderBaseURL: "http://leftover:1/v1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ProviderBaseURL != "" || got.ProviderKind != models.ProviderKindNative {
		t.Errorf("native profile kept provider fields: %+v", got)
	}
}

func TestUpdateProfile_SwitchToNativeClearsProviderFields(t *testing.T) {
	ctrl, _, ctx, codexID, _ := newProviderTestController(t)

	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID:         codexID,
		Name:            "9router",
		Model:           "code",
		ProviderKind:    models.ProviderKindOpenAICompatible,
		ProviderBaseURL: "http://localhost:20128/v1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	native := models.ProviderKindNative
	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, ProviderKind: &native})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ProviderBaseURL != "" || updated.ProviderKind != models.ProviderKindNative {
		t.Errorf("switch to native did not clear provider fields: %+v", updated)
	}
}

// A model change on an existing OpenAI-compatible profile must be re-validated
// even when the request carries no provider fields, or a slash-prefixed model id
// silently routes back to the CLI's built-in vendor provider.
func TestUpdateProfile_ModelChangeRevalidatesProvider(t *testing.T) {
	ctrl, _, ctx, codexID, _ := newProviderTestController(t)

	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID:         codexID,
		Name:            "9router",
		Model:           "code",
		ProviderKind:    models.ProviderKindOpenAICompatible,
		ProviderBaseURL: "http://localhost:20128/v1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	bad := "openai/gpt-4o"
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, Model: &bad}); !errors.Is(err, ErrInvalidProviderConfig) {
		t.Fatalf("model-only update err = %v, want ErrInvalidProviderConfig", err)
	}

	ok := "claude-via-router"
	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, Model: &ok})
	if err != nil {
		t.Fatalf("valid model update: %v", err)
	}
	if !updated.ProviderSupported {
		t.Errorf("ProviderSupported = false on update response, want true")
	}
}

// Update and Duplicate responses must carry ProviderSupported: the frontend
// swaps its cached profile for them and would otherwise hide the provider
// section until the agent list is refetched.
func TestUpdateAndDuplicateProfile_DecorateProviderSupport(t *testing.T) {
	ctrl, _, ctx, codexID, _ := newProviderTestController(t)

	created, err := ctrl.CreateProfile(ctx, CreateProfileRequest{AgentID: codexID, Name: "p", Model: "code"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	name := "p renamed"
	updated, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: created.ID, Name: &name})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !updated.ProviderSupported {
		t.Errorf("UpdateProfile response ProviderSupported = false, want true")
	}

	dup, err := ctrl.DuplicateProfile(ctx, DuplicateProfileRequest{ID: created.ID})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if !dup.ProviderSupported {
		t.Errorf("DuplicateProfile response ProviderSupported = false, want true")
	}
}
