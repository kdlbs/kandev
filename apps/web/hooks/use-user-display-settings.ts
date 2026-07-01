import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateUserSettings } from "@/lib/api";
import { useSearchParams } from "@/lib/routing/client-router";
import { mapSelectedRepositoryIds } from "@/lib/kanban/filters";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { userSettingsQueryOptions } from "@/lib/query/query-options/settings";
import { mapUserSettingsQueryData } from "@/hooks/domains/settings/user-settings-query-data";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { repositoryId, type Repository } from "@/lib/types/http";
import { DEFAULT_TASKS_LIST_GROUP, DEFAULT_TASKS_LIST_SORT } from "@/lib/tasks/tasks-list-options";
import {
  DEFAULT_VOICE_MODE_STATE,
  type UserSettingsState,
} from "@/lib/state/slices/settings/types";

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
  kanbanViewMode?: string | null;
};

function carryForwardTerminalSettings(current: DisplaySettings) {
  return {
    terminalLinkBehavior: (current.terminalLinkBehavior ?? "new_tab") as
      | "new_tab"
      | "browser_panel",
    terminalFontFamily: current.terminalFontFamily ?? null,
    terminalFontSize: current.terminalFontSize ?? null,
  };
}

function carryForwardLspSettings(current: DisplaySettings) {
  return {
    lspAutoStartLanguages: current.lspAutoStartLanguages ?? [],
    lspAutoInstallLanguages: current.lspAutoInstallLanguages ?? [],
    lspServerConfigs: current.lspServerConfigs ?? {},
  };
}

function carryForwardCoreSettings(current: DisplaySettings) {
  return {
    shellOptions: current.shellOptions ?? [],
    defaultEditorId: current.defaultEditorId ?? null,
    chatSubmitKey: current.chatSubmitKey ?? "cmd_enter",
    reviewAutoMarkOnScroll: current.reviewAutoMarkOnScroll ?? true,
    showReleaseNotification: current.showReleaseNotification ?? true,
    releaseNotesLastSeenVersion: current.releaseNotesLastSeenVersion ?? null,
    savedLayouts: current.savedLayouts ?? [],
    defaultUtilityAgentId: current.defaultUtilityAgentId ?? null,
    keyboardShortcuts: current.keyboardShortcuts ?? {},
    tasksListSort: current.tasksListSort ?? DEFAULT_TASKS_LIST_SORT,
    tasksListGroup: current.tasksListGroup ?? DEFAULT_TASKS_LIST_GROUP,
    changesPanelLayout: current.changesPanelLayout ?? "tree",
    systemMetricsDisplay: current.systemMetricsDisplay ?? { showInTopbar: false },
    voiceMode: current.voiceMode ?? { ...DEFAULT_VOICE_MODE_STATE },
  };
}

function carryForwardTaskActionSettings(current: DisplaySettings) {
  return {
    confirmTaskArchive: current.confirmTaskArchive ?? true,
  };
}

function carryForwardSidebarSettings(current: DisplaySettings) {
  return {
    sidebarViews: current.sidebarViews ?? [],
    sidebarActiveViewId: current.sidebarActiveViewId ?? null,
    sidebarDraft: current.sidebarDraft ?? null,
    sidebarTaskPrefs: current.sidebarTaskPrefs ?? {
      pinnedTaskIds: [],
      orderedTaskIds: [],
      subtaskOrderByParentId: {},
    },
  };
}

function emptyTaskCreateLastUsed(): DisplaySettings["taskCreateLastUsed"] {
  return {
    repositoryId: null,
    branch: null,
    agentProfileId: null,
    executorProfileId: null,
  };
}

function hasTaskCreateLastUsed(value: DisplaySettings["taskCreateLastUsed"] | undefined) {
  return Boolean(
    value?.repositoryId || value?.branch || value?.agentProfileId || value?.executorProfileId,
  );
}

function readCachedTaskCreateLastUsed(): DisplaySettings["taskCreateLastUsed"] {
  const cached = {
    repositoryId: getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null),
    branch: getLocalStorage<string | null>(STORAGE_KEYS.LAST_BRANCH, null),
    agentProfileId: getLocalStorage<string | null>(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, null),
    executorProfileId: getLocalStorage<string | null>(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, null),
  };
  return hasTaskCreateLastUsed(cached) ? cached : emptyTaskCreateLastUsed();
}

function resolveTaskCreateLastUsed(current: DisplaySettings) {
  const currentLastUsed = current.taskCreateLastUsed;
  return hasTaskCreateLastUsed(currentLastUsed) ? currentLastUsed : readCachedTaskCreateLastUsed();
}

function carryForwardSyncedLocalSettings(current: DisplaySettings) {
  return {
    taskCreateLastUsed: resolveTaskCreateLastUsed(current),
    jiraSavedViews: current.jiraSavedViews,
    jiraTaskPresets: current.jiraTaskPresets,
    githubSavedPresets: current.githubSavedPresets,
    githubDefaultQueryPresets: current.githubDefaultQueryPresets,
    gitlabSavedPresets: current.gitlabSavedPresets,
  };
}

function carryForwardSettings(current: DisplaySettings) {
  return {
    ...carryForwardCoreSettings(current),
    ...carryForwardTaskActionSettings(current),
    ...carryForwardSidebarSettings(current),
    ...carryForwardSyncedLocalSettings(current),
    ...carryForwardLspSettings(current),
    ...carryForwardTerminalSettings(current),
  };
}

function buildNormalizedSettings(next: CommitPayload, current: DisplaySettings): DisplaySettings {
  return {
    workspaceId: next.workspaceId,
    workflowId: next.workflowId,
    kanbanViewMode:
      next.kanbanViewMode !== undefined ? next.kanbanViewMode : (current.kanbanViewMode ?? null),
    repositoryIds: Array.from(new Set(next.repositoryIds)).sort(),
    preferredShell: next.preferredShell ?? current.preferredShell ?? null,
    enablePreviewOnClick: next.enablePreviewOnClick ?? current.enablePreviewOnClick,
    ...carryForwardSettings(current),
    loaded: true,
  };
}

function isSettingsUnchanged(normalized: DisplaySettings, current: DisplaySettings): boolean {
  if (!current.loaded) return false;
  return (
    normalized.workspaceId === current.workspaceId &&
    normalized.workflowId === current.workflowId &&
    normalized.enablePreviewOnClick === current.enablePreviewOnClick &&
    normalized.kanbanViewMode === current.kanbanViewMode &&
    normalized.repositoryIds.length === current.repositoryIds.length &&
    normalized.repositoryIds.every((id, index) => id === current.repositoryIds[index])
  );
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

function useLoadUserSettings(
  loaded: boolean,
  setUserSettings: (settings: DisplaySettings) => void,
) {
  const query = useQuery({ ...userSettingsQueryOptions(), enabled: !loaded });
  const mapped = useMemo(() => mapUserSettingsQueryData(query.data), [query.data]);

  useEffect(() => {
    if (loaded) return;
    if (!mapped) return;
    setUserSettings(mapped);
  }, [loaded, mapped, setUserSettings]);

  return loaded ? null : mapped;
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
  const loadedUserSettings = useLoadUserSettings(userSettings.loaded, setUserSettings);
  const effectiveUserSettings = loadedUserSettings ?? userSettings;
  const userSettingsRef = useUserSettingsRef(effectiveUserSettings);
  const routeWorkflowId = useSearchParams().get("workflowId");

  const settingsLoadedOnMountRef = useRef(effectiveUserSettings.loaded);

  const commitSettings = useCallback(
    (next: CommitPayload) => {
      const current = userSettingsRef.current;
      const normalized = buildNormalizedSettings(next, current);
      if (isSettingsUnchanged(normalized, current)) return;
      setUserSettings(normalized);
      const payload = {
        workspace_id: normalized.workspaceId ?? "",
        workflow_filter_id: normalized.workflowId ?? "",
        repository_ids: normalized.repositoryIds,
        enable_preview_on_click: normalized.enablePreviewOnClick,
        kanban_view_mode: normalized.kanbanViewMode ?? "",
      };
      persistSettingsPayload(payload);
    },
    [setUserSettings, userSettingsRef],
  );

  useEffect(() => {
    if (!effectiveUserSettings.loaded) return;
    if (routeWorkflowId) return;
    if (settingsLoadedOnMountRef.current) return;
    settingsLoadedOnMountRef.current = true;
    if (effectiveUserSettings.workspaceId && effectiveUserSettings.workspaceId !== workspaceId) {
      onWorkspaceChange?.(effectiveUserSettings.workspaceId);
    }
  }, [
    effectiveUserSettings.loaded,
    effectiveUserSettings.workspaceId,
    onWorkspaceChange,
    routeWorkflowId,
    workspaceId,
  ]);

  useEffect(() => {
    if (!effectiveUserSettings.loaded || !(!effectiveUserSettings.workspaceId && workspaceId)) {
      return;
    }
    queueMicrotask(() => {
      commitSettings({
        workspaceId,
        workflowId: effectiveUserSettings.workflowId,
        repositoryIds: effectiveUserSettings.repositoryIds,
      });
    });
  }, [
    commitSettings,
    effectiveUserSettings.workflowId,
    effectiveUserSettings.loaded,
    effectiveUserSettings.repositoryIds,
    effectiveUserSettings.workspaceId,
    workspaceId,
  ]);

  useEffect(() => {
    if (!effectiveUserSettings.loaded) return;
    if (routeWorkflowId) return;
    if (settingsLoadedOnMountRef.current) return;
    if (effectiveUserSettings.workflowId && effectiveUserSettings.workflowId !== workflowId) {
      onWorkflowChange?.(effectiveUserSettings.workflowId);
    }
  }, [
    effectiveUserSettings.loaded,
    effectiveUserSettings.workflowId,
    workflowId,
    onWorkflowChange,
    routeWorkflowId,
  ]);

  usePruneStaleRepositoryIds(effectiveUserSettings, repositories, commitSettings);

  const allRepositoriesSelected = effectiveUserSettings.repositoryIds.length === 0;
  const selectedRepositoryIds = useMemo(
    () => mapSelectedRepositoryIds(repositories, effectiveUserSettings.repositoryIds),
    [repositories, effectiveUserSettings.repositoryIds],
  );

  return {
    settings: effectiveUserSettings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryIds,
  };
}
