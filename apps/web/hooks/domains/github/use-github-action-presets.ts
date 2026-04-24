"use client";

import { useEffect, useCallback } from "react";
import {
  fetchGitHubActionPresets,
  updateGitHubActionPresets,
  resetGitHubActionPresets,
} from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";
import type { GitHubActionPreset, UpdateGitHubActionPresetsRequest } from "@/lib/types/github";

export function useGitHubActionPresets(workspaceId: string | null) {
  const presets = useAppStore((state) =>
    workspaceId ? (state.actionPresets.byWorkspaceId[workspaceId] ?? null) : null,
  );
  const loading = useAppStore((state) =>
    workspaceId ? Boolean(state.actionPresets.loading[workspaceId]) : false,
  );
  const setPresets = useAppStore((state) => state.setActionPresets);
  const setLoading = useAppStore((state) => state.setActionPresetsLoading);

  useEffect(() => {
    if (!workspaceId) return;
    if (presets || loading) return;
    setLoading(workspaceId, true);
    fetchGitHubActionPresets(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (response) setPresets(workspaceId, response);
      })
      .catch(() => {
        // Leave presets unset on failure; consumers can fall back to defaults.
      })
      .finally(() => {
        setLoading(workspaceId, false);
      });
  }, [workspaceId, presets, loading, setPresets, setLoading]);

  const save = useCallback(
    async (payload: Omit<UpdateGitHubActionPresetsRequest, "workspace_id">) => {
      if (!workspaceId) return null;
      const response = await updateGitHubActionPresets({ workspace_id: workspaceId, ...payload });
      if (response) setPresets(workspaceId, response);
      return response;
    },
    [workspaceId, setPresets],
  );

  const reset = useCallback(async () => {
    if (!workspaceId) return null;
    const response = await resetGitHubActionPresets(workspaceId);
    if (response) setPresets(workspaceId, response);
    return response;
  }, [workspaceId, setPresets]);

  const savePR = useCallback(async (pr: GitHubActionPreset[]) => save({ pr }), [save]);

  const saveIssue = useCallback(async (issue: GitHubActionPreset[]) => save({ issue }), [save]);

  return {
    presets,
    loading,
    save,
    savePR,
    saveIssue,
    reset,
  };
}
