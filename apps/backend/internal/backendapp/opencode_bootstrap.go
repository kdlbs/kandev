package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

const openCodeAgentID = "opencode-acp"

type openCodeProfileLister interface {
	ListAgentProfiles(context.Context, string) ([]*models.AgentProfile, error)
}

type openCodeSessionEvidence interface {
	HasTaskSessionsByAgentProfile(context.Context, string) (bool, error)
}

type openCodeNativeDetector func(context.Context) (agents.OpenCodeNativeRuntime, bool, error)

func bootstrapOpenCodeSelection(
	ctx context.Context,
	store *managedruntime.Store,
	repos *Repositories,
	agentRegistry *registry.Registry,
	log *logger.Logger,
) error {
	if store == nil {
		return errors.New("required runtime-selection store is unavailable")
	}
	defaults, err := openCodeRuntimeDefaults()
	if err != nil {
		return err
	}
	_, found, err := store.GetOpenCodeSelection(ctx)
	if err != nil {
		return err
	}
	var evidence managedruntime.OpenCodeBootstrapEvidence
	if !found {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve user home for prior-use evidence: %w", err)
		}
		var profiles openCodeProfileLister
		var sessions openCodeSessionEvidence
		if repos != nil {
			profiles = repos.AgentSettings
			sessions = repos.Task
		}
		evidence, err = collectOpenCodeBootstrapEvidence(
			ctx,
			profiles,
			sessions,
			home,
			os.Getenv("OPENCODE_DB"),
			agents.DetectOpenCodeNativeRuntime,
		)
		if err != nil {
			return err
		}
	}
	selection, err := store.BootstrapOpenCode(ctx, defaults, evidence)
	if err != nil {
		return err
	}
	if agentRegistry != nil {
		agentRegistry.SetManagedRuntimeSelectionStore(store)
	}
	if log != nil {
		log.Info("OpenCode runtime selection loaded",
			zap.String("family", string(selection.Family)),
			zap.String("source", string(selection.Source)),
			zap.String("package", selection.Package),
			zap.Uint64("revision", selection.Revision),
		)
	}
	return nil
}

func openCodeRuntimeDefaults() (managedruntime.OpenCodeRuntimeDefaults, error) {
	v1, err := agents.NewOpenCodeACP().ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV1)
	if err != nil {
		return managedruntime.OpenCodeRuntimeDefaults{}, err
	}
	v2, err := agents.NewOpenCodeACP().ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		return managedruntime.OpenCodeRuntimeDefaults{}, err
	}
	return managedruntime.OpenCodeRuntimeDefaults{
		V1Package: v1.Package,
		V1Version: v1.DefaultVersionOrPinned(),
		V2Package: v2.Package,
		V2Version: v2.DefaultVersionOrPinned(),
	}, nil
}

func collectOpenCodeBootstrapEvidence(
	ctx context.Context,
	profiles openCodeProfileLister,
	sessions openCodeSessionEvidence,
	home string,
	databaseOverride string,
	detectNative openCodeNativeDetector,
) (managedruntime.OpenCodeBootstrapEvidence, error) {
	var evidence managedruntime.OpenCodeBootstrapEvidence
	if detectNative != nil {
		native, found, err := detectNative(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return evidence, ctx.Err()
			}
			// A broken native CLI is evidence of prior use, but it cannot safely
			// select a native family. Keep the established v1 fallback.
			evidence.PriorUse = true
		} else if found {
			evidence.NativeFamily = native.Family
		}
	}
	if hasOpenCodeFilesystemEvidence(home, databaseOverride) {
		evidence.PriorUse = true
	}
	if profiles == nil || sessions == nil {
		return evidence, nil
	}
	items, err := profiles.ListAgentProfiles(ctx, openCodeAgentID)
	if err != nil {
		return evidence, fmt.Errorf("list OpenCode profiles for prior-use evidence: %w", err)
	}
	for _, profile := range items {
		if profile == nil || profile.ID == "" {
			continue
		}
		hasSessions, err := sessions.HasTaskSessionsByAgentProfile(ctx, profile.ID)
		if err != nil {
			return evidence, fmt.Errorf("query OpenCode session history: %w", err)
		}
		if hasSessions {
			evidence.PriorUse = true
			break
		}
	}
	return evidence, nil
}

func hasOpenCodeFilesystemEvidence(home, databaseOverride string) bool {
	paths := []string{
		filepath.Join(home, ".config", "opencode", "opencode.json"),
		filepath.Join(home, ".config", "opencode", "opencode.jsonc"),
		filepath.Join(home, ".local", "share", "opencode", "opencode.db"),
	}
	if databaseOverride != "" {
		paths = append(paths, databaseOverride)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}
