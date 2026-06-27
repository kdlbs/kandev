"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { updateActionPresets, resetActionPresets } from "@/lib/api/domains/gitlab-api";
import { qk } from "@/lib/query/keys";
import { gitlabActionPresetsQueryOptions } from "@/lib/query/query-options/gitlab";
import type { GitLabActionPreset } from "@/lib/types/gitlab";

/**
 * useGitLabActionPresets fetches the workspace's stored presets (falling back
 * to defaults server-side) and exposes update/reset helpers. The per-workspace
 * attempted set guards against an infinite re-fetch loop when the API is
 * unreachable.
 */
export function useGitLabActionPresets(workspaceId: string | null | undefined) {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...gitlabActionPresetsQueryOptions(workspaceId ?? ""),
    enabled: Boolean(workspaceId),
  });
  const presets = query.data ?? null;

  const update = useCallback(
    async (body: { mr?: GitLabActionPreset[]; issue?: GitLabActionPreset[] }) => {
      if (!workspaceId) return null;
      const result = await updateActionPresets(workspaceId, body);
      if (result) {
        queryClient.setQueryData(qk.integrations.gitlab.actionPresets(workspaceId), result);
      }
      return result;
    },
    [queryClient, workspaceId],
  );

  const reset = useCallback(async () => {
    if (!workspaceId) return null;
    const result = await resetActionPresets(workspaceId);
    if (result) {
      queryClient.setQueryData(qk.integrations.gitlab.actionPresets(workspaceId), result);
    }
    return result;
  }, [queryClient, workspaceId]);

  return {
    presets,
    loading: query.isFetching && !query.isSuccess,
    update,
    reset,
  };
}
