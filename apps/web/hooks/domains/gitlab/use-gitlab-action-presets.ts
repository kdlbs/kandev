"use client";

import { useEffect, useCallback, useRef } from "react";
import {
  getActionPresets,
  updateActionPresets,
  resetActionPresets,
} from "@/lib/api/domains/gitlab-api";
import { useAppStore } from "@/components/state-provider";
import type { GitLabActionPreset } from "@/lib/types/gitlab";

/**
 * useGitLabActionPresets fetches the workspace's stored presets (falling back
 * to defaults server-side) and exposes update/reset helpers. The per-workspace
 * attempted set guards against an infinite re-fetch loop when the API is
 * unreachable.
 */
export function useGitLabActionPresets(workspaceId: string | null | undefined) {
  const presets = useAppStore((state) =>
    workspaceId ? state.gitlabActionPresets.byWorkspaceId[workspaceId] : null,
  );
  const loading = useAppStore((state) => state.gitlabActionPresets.loading);
  const set = useAppStore((state) => state.setGitLabActionPresets);
  const setLoading = useAppStore((state) => state.setGitLabActionPresetsLoading);
  const attemptedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!workspaceId || presets || loading) return;
    if (attemptedRef.current.has(workspaceId)) return;
    attemptedRef.current.add(workspaceId);
    setLoading(true);
    getActionPresets(workspaceId)
      .then((res) => {
        if (res) set(workspaceId, res);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [workspaceId, presets, loading, set, setLoading]);

  const update = useCallback(
    async (body: { mr?: GitLabActionPreset[]; issue?: GitLabActionPreset[] }) => {
      if (!workspaceId) return null;
      const result = await updateActionPresets(workspaceId, body);
      if (result) set(workspaceId, result);
      return result;
    },
    [workspaceId, set],
  );

  const reset = useCallback(async () => {
    if (!workspaceId) return null;
    const result = await resetActionPresets(workspaceId);
    if (result) set(workspaceId, result);
    return result;
  }, [workspaceId, set]);

  return { presets, loading, update, reset };
}
