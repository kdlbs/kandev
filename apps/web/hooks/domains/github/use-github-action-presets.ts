"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { updateGitHubActionPresets, resetGitHubActionPresets } from "@/lib/api/domains/github-api";
import { githubQueryOptions } from "@/lib/query/query-options/github";
import { qk } from "@/lib/query/keys";
import type { UpdateGitHubActionPresetsRequest } from "@/lib/types/github";

export function useGitHubActionPresets(workspaceId: string | null) {
  const qc = useQueryClient();
  const { data: presets, isLoading } = useQuery(githubQueryOptions.actionPresets(workspaceId));

  const saveMutation = useMutation({
    mutationFn: (payload: Omit<UpdateGitHubActionPresetsRequest, "workspace_id">) => {
      if (!workspaceId) return Promise.reject(new Error("No workspace"));
      return updateGitHubActionPresets({ workspace_id: workspaceId, ...payload });
    },
    onSuccess: (response) => {
      if (!workspaceId || !response) return;
      qc.setQueryData(qk.github.actionPresets(workspaceId), response);
    },
    onSettled: () => {
      if (workspaceId) {
        void qc.invalidateQueries({ queryKey: qk.github.actionPresets(workspaceId) });
      }
    },
  });

  const resetMutation = useMutation({
    mutationFn: () => {
      if (!workspaceId) return Promise.reject(new Error("No workspace"));
      return resetGitHubActionPresets(workspaceId);
    },
    onSuccess: (response) => {
      if (!workspaceId || !response) return;
      qc.setQueryData(qk.github.actionPresets(workspaceId), response);
    },
    onSettled: () => {
      if (workspaceId) {
        void qc.invalidateQueries({ queryKey: qk.github.actionPresets(workspaceId) });
      }
    },
  });

  return {
    presets: presets ?? null,
    loading: isLoading,
    save: (payload: Omit<UpdateGitHubActionPresetsRequest, "workspace_id">) =>
      saveMutation.mutateAsync(payload),
    reset: () => resetMutation.mutateAsync(),
  };
}
