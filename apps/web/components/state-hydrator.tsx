"use client";

import { useLayoutEffect } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useAppStoreApi } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import { prepareResultToSessionState } from "@/lib/state/slices/session-runtime/prepare-result";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";
import type {
  Agent,
  AgentDiscovery,
  AvailableAgent,
  Executor,
  ListWorkspacesResponse,
  ListRepositoriesResponse,
  TaskSession,
  TaskSessionsResponse,
  ToolStatus,
} from "@/lib/types/http";
import type { KanbanMultiData, WorkflowsListData } from "@/lib/query/query-options/kanban";
import type { SsrInitialState } from "@/lib/ssr/initial-state";
import type { AgentProfileOption } from "@/lib/types/settings";

/**
 * Transitional SSR→TQ bridge for the workspace domain. This page hydrates
 * Zustand (no TQ HydrationBoundary), so seed the workspaces + per-workspace
 * repository queries from the SSR snapshot. Without it, consumers reading
 * qk.workspaces.* mount with an empty cache and flash empty repo/workspace
 * pickers until the client refetch lands. Seed-if-absent so a live WS/refetch
 * result is never clobbered.
 */
function seedWorkspaceQueries(queryClient: QueryClient, initialState: SsrInitialState): void {
  const workspaceItems = initialState.workspaces?.items;
  if (workspaceItems?.length && !queryClient.getQueryData(qk.workspaces.all())) {
    queryClient.setQueryData<ListWorkspacesResponse>(qk.workspaces.all(), {
      workspaces: workspaceItems as ListWorkspacesResponse["workspaces"],
      total: workspaceItems.length,
    });
  }

  const reposByWorkspace = initialState.repositories?.itemsByWorkspaceId;
  if (reposByWorkspace) {
    for (const [wsId, repos] of Object.entries(reposByWorkspace)) {
      if (!queryClient.getQueryData(qk.workspaces.repos(wsId))) {
        queryClient.setQueryData<ListRepositoriesResponse>(qk.workspaces.repos(wsId), {
          repositories: repos,
          total: repos.length,
        });
      }
    }
  }
}

/**
 * Transitional SSR→TQ bridge for the kanban domain. SSR builds the active
 * task's single-workflow snapshot (`initialState.kanban`) and the workspace
 * workflows list (`initialState.workflows.items`) into the Zustand payload.
 * Seed both TQ caches from it so task-detail consumers (useTask, useTaskById,
 * usePlanActions, mention items) and workflow pickers don't mount empty and
 * flash before the client refetch lands. Seed-if-absent so a live WS/refetch
 * result is never clobbered.
 */
function seedKanbanQueries(queryClient: QueryClient, initialState: SsrInitialState): void {
  const workspaceId = initialState.workspaces?.activeId ?? undefined;
  const workflows = initialState.workflows?.items;
  if (workspaceId && workflows?.length) {
    const key = qk.kanban.workflowsList(workspaceId);
    if (!queryClient.getQueryData(key)) {
      queryClient.setQueryData<WorkflowsListData>(key, workflows as WorkflowsListData);
    }
  }

  const kanban = initialState.kanban;
  const wfId = kanban?.workflowId;
  if (wfId && (kanban.steps.length || kanban.tasks.length)) {
    const workflowName = workflows?.find((w) => w.id === wfId)?.name ?? wfId;
    queryClient.setQueryData<KanbanMultiData>(qk.kanban.multi(), (prev) => {
      // Seed-if-absent at the per-workflow granularity: never clobber a snapshot
      // a live fetch/WS event already wrote, but do add this workflow if missing.
      if (prev?.snapshots[wfId]) return prev;
      const snapshot = { workflowId: wfId, workflowName, steps: kanban.steps, tasks: kanban.tasks };
      if (!prev) return { snapshots: { [wfId]: snapshot } };
      return { ...prev, snapshots: { ...prev.snapshots, [wfId]: snapshot } };
    });
  }
}

/**
 * Transitional SSR→TQ bridge for the settings domain. This page hydrates
 * Zustand (no TQ HydrationBoundary), so seed the agents, agent-profiles, and
 * user-settings queries from the SSR snapshot. Without it, consumers reading
 * qk.settings.* (model/mode pickers, sessions dropdown, chat submit key, etc.)
 * mount with an empty cache and flash before the client refetch lands.
 * Seed-if-absent so a live WS/refetch result is never clobbered.
 */
/** Seed a TQ key from an SSR value only when it is non-empty and not already cached. */
function seedIfAbsent<T>(
  queryClient: QueryClient,
  queryKey: readonly unknown[],
  value: T | undefined,
  hasContent: (v: T) => boolean,
): void {
  if (value && hasContent(value) && !queryClient.getQueryData(queryKey)) {
    queryClient.setQueryData<T>(queryKey, value);
  }
}

function seedSettingsQueries(queryClient: QueryClient, initialState: SsrInitialState): void {
  const has = <T,>(arr: T[]) => arr.length > 0;
  seedIfAbsent<Agent[]>(queryClient, qk.settings.agents(), initialState.settingsAgents?.items, has);
  seedIfAbsent<AgentProfileOption[]>(
    queryClient,
    qk.settings.agentProfiles(),
    initialState.agentProfiles?.items,
    has,
  );
  seedIfAbsent(
    queryClient,
    qk.settings.userSettings(),
    initialState.userSettings,
    (us) => us.loaded,
  );
  seedIfAbsent<Executor[]>(
    queryClient,
    qk.settings.executors(),
    initialState.executors?.items,
    has,
  );
  seedIfAbsent<AgentDiscovery[]>(
    queryClient,
    qk.settings.agentDiscovery(),
    initialState.agentDiscovery?.items,
    has,
  );
  const available = initialState.availableAgents;
  seedIfAbsent<{ agents: AvailableAgent[]; tools: ToolStatus[] }>(
    queryClient,
    qk.settings.availableAgents(),
    available ? { agents: available.items, tools: available.tools } : undefined,
    (v) => v.agents.length > 0,
  );
}

/**
 * Transitional SSR→TQ bridge for the session (taskSessions) domain. This page
 * hydrates Zustand (no TQ HydrationBoundary), so seed BOTH the per-task session
 * list (`qk.taskSession.byTask`) and the new per-session by-id observe surface
 * (`qk.taskSession.byId`) from the SSR snapshot. Without it, by-id / by-task
 * consumers mount with an empty cache and flash empty session panels until the
 * client refetch or the first WS event lands. Seed-if-absent so a live
 * WS/refetch result is never clobbered.
 */
function seedSessionQueries(queryClient: QueryClient, initialState: SsrInitialState): void {
  const byTask = initialState.taskSessionsByTask?.itemsByTaskId;
  if (byTask) {
    for (const [taskId, sessions] of Object.entries(byTask)) {
      const key = qk.taskSession.byTask(taskId);
      if (sessions.length && !queryClient.getQueryData(key)) {
        queryClient.setQueryData<TaskSessionsResponse>(key, {
          sessions,
          total: sessions.length,
        });
      }
    }
  }

  const byId = initialState.taskSessions?.items;
  if (byId) {
    for (const session of Object.values(byId)) {
      const key = qk.taskSession.byId(session.id);
      if (!queryClient.getQueryData(key)) {
        queryClient.setQueryData<TaskSession>(key, session);
      }
    }
  }
}

/**
 * Transitional SSR→TQ bridge for the prepare-progress (D6) cache. Derives the
 * per-session prepare state from each SSR session's `metadata.prepare_result`
 * and seeds `qk.session.prepareProgress`. Without it, the prepare panel +
 * `usePrepareSummary` / `useSessionState` readers (now `useQuery`-backed) mount
 * with an empty cache and flash an empty "Environment prepared" row until the
 * first live executor.prepare.* event lands. Seed-if-absent so a live WS event
 * is never clobbered.
 */
function seedPrepareProgressQueries(queryClient: QueryClient, initialState: SsrInitialState): void {
  const byId = initialState.taskSessions?.items;
  if (!byId) return;
  for (const session of Object.values(byId)) {
    const key = qk.session.prepareProgress(session.id);
    if (queryClient.getQueryData(key)) continue;
    const prepareState = prepareResultToSessionState(session.id, session.metadata);
    if (prepareState) {
      queryClient.setQueryData<SessionPrepareState>(key, prepareState);
    }
  }
}

type StateHydratorProps = {
  initialState: SsrInitialState;
  /** Session ID to force-merge even if it's active (for navigation refresh) */
  sessionId?: string;
};

export function StateHydrator({ initialState, sessionId }: StateHydratorProps) {
  const store = useAppStoreApi();
  const queryClient = useQueryClient();

  // Use useLayoutEffect to hydrate state synchronously before child effects run.
  // This ensures SSR-hydrated data is available before hooks like useSettingsData
  // decide whether to fetch data.
  useLayoutEffect(() => {
    if (Object.keys(initialState).length) {
      store.getState().hydrate(initialState, {
        forceMergeSessionId: sessionId,
      });
    }

    // Transitional SSR→TQ bridge for the turns domain: this page hydrates
    // Zustand (no TQ HydrationBoundary), so seed the turns query from the SSR
    // snapshot. Without it, consumers reading qk.session.turns mount with an
    // empty cache and flash a zeroed elapsed timer until the client refetch
    // lands. Seed-if-absent so a live WS/refetch result is never clobbered.
    const turns = sessionId ? initialState.turns?.bySession?.[sessionId] : undefined;
    if (sessionId && turns && !queryClient.getQueryData(qk.session.turns(sessionId))) {
      queryClient.setQueryData(qk.session.turns(sessionId), {
        turns,
        activeTurnId: initialState.turns?.activeBySession?.[sessionId] ?? null,
      });
    }

    seedWorkspaceQueries(queryClient, initialState);
    seedKanbanQueries(queryClient, initialState);
    seedSettingsQueries(queryClient, initialState);
    seedSessionQueries(queryClient, initialState);
    seedPrepareProgressQueries(queryClient, initialState);
  }, [initialState, sessionId, store, queryClient]);

  return null;
}
