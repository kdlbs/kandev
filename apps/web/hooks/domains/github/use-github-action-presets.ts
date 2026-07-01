"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { updateGitHubActionPresets, resetGitHubActionPresets } from "@/lib/api/domains/github-api";
import { qk } from "@/lib/query/keys";
import { githubActionPresetsQueryOptions } from "@/lib/query/query-options/github";
import type { UpdateGitHubActionPresetsRequest } from "@/lib/types/github";

export function useGitHubActionPresets(workspaceId: string | null) {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...githubActionPresetsQueryOptions(workspaceId ?? ""),
    enabled: Boolean(workspaceId),
  });
  const presets = query.data ?? null;

  const save = useCallback(
    async (payload: Omit<UpdateGitHubActionPresetsRequest, "workspace_id">) => {
      if (!workspaceId) return null;
      const response = await updateGitHubActionPresets({ workspace_id: workspaceId, ...payload });
      if (response) {
        queryClient.setQueryData(qk.integrations.github.actionPresets(workspaceId), response);
      }
      return response;
    },
    [queryClient, workspaceId],
  );

  const reset = useCallback(async () => {
    if (!workspaceId) return null;
    const response = await resetGitHubActionPresets(workspaceId);
    if (response) {
      queryClient.setQueryData(qk.integrations.github.actionPresets(workspaceId), response);
    }
    return response;
  }, [queryClient, workspaceId]);

  return {
    presets,
    loading: query.isFetching && !query.isSuccess,
    save,
    reset,
  };
}
