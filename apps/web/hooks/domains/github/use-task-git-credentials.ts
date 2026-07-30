"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import type { TaskGitCredentialsMode } from "@/lib/types/github";

export type TaskGitCredentialsState = {
  mode: TaskGitCredentialsMode;
  loading: boolean;
  error: boolean;
  save: (mode: TaskGitCredentialsMode) => Promise<void>;
};

export function useTaskGitCredentials(workspaceId: string): TaskGitCredentialsState {
  const [mode, setMode] = useState<TaskGitCredentialsMode>("managed");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const activeWorkspaceId = useRef(workspaceId);
  activeWorkspaceId.current = workspaceId;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(false);
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (!cancelled) setMode(settings.task_git_credentials_mode ?? "managed");
      })
      .catch(() => {
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  const save = useCallback(
    async (nextMode: TaskGitCredentialsMode) => {
      const updated = await updateGitHubWorkspaceSettings({
        workspace_id: workspaceId,
        task_git_credentials_mode: nextMode,
      });
      if (activeWorkspaceId.current !== workspaceId) return;
      setMode(updated.task_git_credentials_mode ?? "managed");
      setError(false);
    },
    [workspaceId],
  );

  return { mode, loading, error, save };
}
