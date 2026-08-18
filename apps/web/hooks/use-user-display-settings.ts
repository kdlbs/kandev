import { useCallback, useEffect, useMemo, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateUserSettings } from "@/lib/api";
import { useSearchParams } from "@/lib/routing/client-router";
import { mapSelectedRepositoryIds } from "@/lib/kanban/filters";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useEnsureUserSettings } from "@/hooks/use-ensure-user-settings";
import { repositoryId, type Repository } from "@/lib/types/http";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";

type DisplaySettings = UserSettingsState;

type UseUserDisplaySettingsInput = {
  workspaceId: string | null;
  workflowId: string | null;
  onWorkspaceChange?: (workspaceId: string | null) => void;
  onWorkflowChange?: (workflowId: string | null) => void;
};

type CommitPayload = {
  workspaceId: string | null;
  workflowId: string | null;
  repositoryIds: string[];
  preferredShell?: string | null;
  enablePreviewOnClick?: boolean;
  tasksListShowDetails?: boolean;
  kanbanViewMode?: string | null;
  hiddenWorkflowStepIds?: Record<string, string[]>;
  workflowIdsWithAutoHideEmptySteps?: string[];
};

export function normalizeWorkflowIds(ids: string[]): string[] {
  return Array.from(new Set(ids)).sort();
}

export function normalizeHiddenStepIds(raw: Record<string, string[]>): Record<string, string[]> {
  const result: Record<string, string[]> = {};
  for (const [workflowId, ids] of Object.entries(raw)) {
    const sorted = Array.from(new Set(ids)).sort();
    if (sorted.length > 0) {
      result[workflowId] = sorted;
    }
  }
  return result;
}

function hiddenStepIdsEqual(a: Record<string, string[]>, b: Record<string, string[]>): boolean {
  const aKeys = Object.keys(a).sort();
  const bKeys = Object.keys(b).sort();
  if (aKeys.length !== bKeys.length) return false;
  return aKeys.every((key, i) => {
    if (key !== bKeys[i]) return false;
    const aIds = [...a[key]].sort();
    const bIds = [...b[key]].sort();
    return aIds.length === bIds.length && aIds.every((id, j) => id === bIds[j]);
  });
}

function buildNormalizedSettings(next: CommitPayload, current: DisplaySettings): DisplaySettings {
  return {
    ...current,
    workspaceId: next.workspaceId,
    workflowId: next.workflowId,
    repositoryIds: Array.from(new Set(next.repositoryIds)).sort(),
    preferredShell: next.preferredShell ?? current.preferredShell,
    enablePreviewOnClick: next.enablePreviewOnClick ?? current.enablePreviewOnClick,
    tasksListShowDetails: next.tasksListShowDetails ?? current.tasksListShowDetails,
    hiddenWorkflowStepIds: normalizeHiddenStepIds(
      next.hiddenWorkflowStepIds ?? current.hiddenWorkflowStepIds ?? {},
    ),
    workflowIdsWithAutoHideEmptySteps: normalizeWorkflowIds(
      next.workflowIdsWithAutoHideEmptySteps ?? current.workflowIdsWithAutoHideEmptySteps ?? [],
    ),
    loaded: true,
  };
}

export function isSettingsUnchanged(
  normalized: DisplaySettings,
  current: DisplaySettings,
): boolean {
  if (!current.loaded) return false;
  return (
    normalized.workspaceId === current.workspaceId &&
    normalized.workflowId === current.workflowId &&
    normalized.enablePreviewOnClick === current.enablePreviewOnClick &&
    normalized.tasksListShowDetails === current.tasksListShowDetails &&
    normalized.repositoryIds.length === current.repositoryIds.length &&
    normalized.repositoryIds.every((id, index) => id === current.repositoryIds[index]) &&
    hiddenStepIdsEqual(
      normalized.hiddenWorkflowStepIds ?? {},
      current.hiddenWorkflowStepIds ?? {},
    ) &&
    normalizeWorkflowIds(normalized.workflowIdsWithAutoHideEmptySteps ?? []).join("\0") ===
      normalizeWorkflowIds(current.workflowIdsWithAutoHideEmptySteps ?? []).join("\0")
  );
}

export function buildSettingsUpdatePayload(normalized: DisplaySettings): Record<string, unknown> {
  return {
    workspace_id: normalized.workspaceId ?? "",
    workflow_filter_id: normalized.workflowId ?? "",
    repository_ids: normalized.repositoryIds,
    enable_preview_on_click: normalized.enablePreviewOnClick,
    tasks_list_show_details: normalized.tasksListShowDetails,
    kanban_hidden_step_ids: normalized.hiddenWorkflowStepIds,
    workflow_ids_with_auto_hide_empty_steps: normalized.workflowIdsWithAutoHideEmptySteps,
  };
}

function persistSettingsPayload(payload: Record<string, unknown>) {
  const client = getWebSocketClient();
  if (!client) {
    updateUserSettings(payload, { cache: "no-store" }).catch(() => {
      /* ignore */
    });
    return;
  }
  client.request("user.settings.update", payload).catch(() => {
    updateUserSettings(payload, { cache: "no-store" }).catch(() => {
      /* ignore */
    });
  });
}

function useUserSettingsRef(userSettings: DisplaySettings) {
  const userSettingsRef = useRef(userSettings);
  useEffect(() => {
    userSettingsRef.current = userSettings;
  }, [userSettings]);
  return userSettingsRef;
}

function usePruneStaleRepositoryIds(
  userSettings: DisplaySettings,
  repositories: Repository[],
  commitSettings: (next: CommitPayload) => void,
) {
  useEffect(() => {
    if (!userSettings.loaded || repositories.length === 0) return;
    const repoIds = repositories.map((repo: Repository) => repo.id);
    const validIds = userSettings.repositoryIds.filter((id: string) =>
      repoIds.includes(repositoryId(id)),
    );
    const isSame =
      validIds.length === userSettings.repositoryIds.length &&
      validIds.every((id: string, index: number) => id === userSettings.repositoryIds[index]);
    if (!isSame) {
      queueMicrotask(() => {
        commitSettings({
          workspaceId: userSettings.workspaceId,
          workflowId: userSettings.workflowId,
          repositoryIds: validIds,
        });
      });
    }
  }, [
    commitSettings,
    repositories,
    userSettings.workflowId,
    userSettings.loaded,
    userSettings.repositoryIds,
    userSettings.workspaceId,
  ]);
}

export function useUserDisplaySettings({
  workspaceId,
  workflowId,
  onWorkspaceChange,
  onWorkflowChange,
}: UseUserDisplaySettingsInput) {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, true);
  const userSettingsRef = useUserSettingsRef(userSettings);
  const routeWorkflowId = useSearchParams().get("workflowId");

  const settingsLoadedOnMountRef = useRef(userSettings.loaded);

  const commitSettings = useCallback(
    (next: CommitPayload) => {
      const current = userSettingsRef.current;
      const normalized = buildNormalizedSettings(next, current);
      if (isSettingsUnchanged(normalized, current)) return;
      setUserSettings(normalized);
      persistSettingsPayload(buildSettingsUpdatePayload(normalized));
    },
    [setUserSettings, userSettingsRef],
  );

  useEnsureUserSettings();

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (routeWorkflowId) return;
    if (settingsLoadedOnMountRef.current) return;
    settingsLoadedOnMountRef.current = true;
    if (userSettings.workspaceId && userSettings.workspaceId !== workspaceId) {
      onWorkspaceChange?.(userSettings.workspaceId);
    }
  }, [
    onWorkspaceChange,
    routeWorkflowId,
    userSettings.loaded,
    userSettings.workspaceId,
    workspaceId,
  ]);

  useEffect(() => {
    if (!userSettings.loaded || !(!userSettings.workspaceId && workspaceId)) return;
    queueMicrotask(() => {
      commitSettings({
        workspaceId,
        workflowId: userSettings.workflowId,
        repositoryIds: userSettings.repositoryIds,
      });
    });
  }, [
    commitSettings,
    userSettings.workflowId,
    userSettings.loaded,
    userSettings.repositoryIds,
    userSettings.workspaceId,
    workspaceId,
  ]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (routeWorkflowId) return;
    if (settingsLoadedOnMountRef.current) return;
    if (userSettings.workflowId && userSettings.workflowId !== workflowId) {
      onWorkflowChange?.(userSettings.workflowId);
    }
  }, [workflowId, onWorkflowChange, routeWorkflowId, userSettings.workflowId, userSettings.loaded]);

  usePruneStaleRepositoryIds(userSettings, repositories, commitSettings);

  const allRepositoriesSelected = userSettings.repositoryIds.length === 0;
  const selectedRepositoryIds = useMemo(
    () => mapSelectedRepositoryIds(repositories, userSettings.repositoryIds),
    [repositories, userSettings.repositoryIds],
  );

  return {
    settings: userSettings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryIds,
  };
}
