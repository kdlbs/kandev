"use client";

import { useCallback, useEffect } from "react";
import type { StoreApi } from "zustand";
import {
  archiveAgentProject,
  createAgentProject,
  deleteAgentProject,
  listAgentProjects,
  restoreAgentProject,
  updateAgentProject,
  type CreateAgentProjectPayload,
  type UpdateAgentProjectPayload,
} from "@/lib/api/domains/agent-projects-api";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import type { AppState } from "@/lib/state/store";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { subscribeAgentProjectTaskEvents } from "@/lib/ws/handlers/agent-project-events";
import {
  selectAgentProjects,
  selectAgentProjectsError,
  selectAgentProjectsLoaded,
  selectAgentProjectsLoading,
} from "@/lib/state/slices/agent-projects/selectors";

const inFlightLoads = new WeakMap<StoreApi<AppState>, Map<string, Promise<void>>>();
const queuedRefreshes = new WeakMap<StoreApi<AppState>, Map<string, Promise<void>>>();
const loadSequence = new WeakMap<StoreApi<AppState>, Map<string, number>>();

function collectionKey(workspaceId: string, archived: boolean): string {
  return `${workspaceId}:${archived ? "archived" : "active"}`;
}

function queueForcedRefresh(
  store: StoreApi<AppState>,
  workspaceId: string,
  archived: boolean,
  key: string,
  existing: Promise<void>,
): Promise<void> {
  let queued = queuedRefreshes.get(store);
  if (!queued) {
    queued = new Map();
    queuedRefreshes.set(store, queued);
  }
  const queuedRefresh = queued.get(key);
  if (queuedRefresh) return queuedRefresh;
  const refresh = existing
    .then(() => loadAgentProjects(store, workspaceId, archived, true))
    .finally(() => {
      if (queued?.get(key) === refresh) queued.delete(key);
    });
  queued.set(key, refresh);
  return refresh;
}

export function loadAgentProjects(
  store: StoreApi<AppState>,
  workspaceId: string,
  archived = false,
  force = false,
): Promise<void> {
  let pending = inFlightLoads.get(store);
  if (!pending) {
    pending = new Map();
    inFlightLoads.set(store, pending);
  }
  const key = collectionKey(workspaceId, archived);
  const existing = pending.get(key);
  if (existing) {
    return force ? queueForcedRefresh(store, workspaceId, archived, key, existing) : existing;
  }

  let sequences = loadSequence.get(store);
  if (!sequences) {
    sequences = new Map();
    loadSequence.set(store, sequences);
  }
  const sequence = (sequences.get(key) ?? 0) + 1;
  sequences.set(key, sequence);
  if (
    !force &&
    store.getState().agentProjects.active.loadedByWorkspaceId[workspaceId] &&
    !archived
  ) {
    return Promise.resolve();
  }
  if (
    !force &&
    store.getState().agentProjects.archived.loadedByWorkspaceId[workspaceId] &&
    archived
  ) {
    return Promise.resolve();
  }
  store.getState().setAgentProjectsLoading(workspaceId, true, archived);

  const request = listAgentProjects(workspaceId, archived, { cache: "no-store" })
    .then((response) => {
      if (sequences?.get(key) !== sequence) return;
      store.getState().setAgentProjects(workspaceId, response.projects ?? [], archived);
    })
    .catch((error: unknown) => {
      if (sequences?.get(key) !== sequence) return;
      store
        .getState()
        .setAgentProjectsError(
          workspaceId,
          error instanceof Error ? error.message : String(error),
          archived,
        );
    })
    .finally(() => {
      if (pending?.get(key) === request) pending.delete(key);
    });
  pending.set(key, request);
  return request;
}

export function useAgentProjects(
  workspaceId: string | null | undefined,
  archived = false,
  enabled = true,
) {
  const store = useAppStoreApi();
  const projects = useAppStore((state) => selectAgentProjects(state, workspaceId, archived));
  const loaded = useAppStore((state) => selectAgentProjectsLoaded(state, workspaceId, archived));
  const loading = useAppStore((state) => selectAgentProjectsLoading(state, workspaceId, archived));
  const error = useAppStore((state) => selectAgentProjectsError(state, workspaceId, archived));
  const refresh = useCallback(async () => {
    if (workspaceId) await loadAgentProjects(store, workspaceId, archived, true);
  }, [archived, store, workspaceId]);

  useEffect(() => {
    if (enabled && workspaceId && !loaded) void loadAgentProjects(store, workspaceId, archived);
  }, [archived, enabled, loaded, store, workspaceId]);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    let disposed = false;
    let refreshing = false;
    let refreshAgain = false;
    const refreshAfterProjectTaskEvent = async () => {
      if (refreshing) {
        refreshAgain = true;
        return;
      }
      refreshing = true;
      try {
        do {
          refreshAgain = false;
          await loadAgentProjects(store, workspaceId, archived, true);
        } while (refreshAgain && !disposed);
      } finally {
        refreshing = false;
      }
    };
    const unsubscribe = subscribeAgentProjectTaskEvents((event) => {
      if (event.workspaceId === workspaceId) void refreshAfterProjectTaskEvent();
    });
    return () => {
      disposed = true;
      unsubscribe();
    };
  }, [archived, enabled, store, workspaceId]);

  return { projects, loaded, loading, error, refresh };
}

export function useAgentProjectMutations() {
  const store = useAppStoreApi();
  const refresh = useCallback(
    async (workspaceId: string) => {
      await Promise.all([
        loadAgentProjects(store, workspaceId, false, true),
        loadAgentProjects(store, workspaceId, true, true),
      ]);
    },
    [store],
  );

  return {
    async create(workspaceId: string, payload: CreateAgentProjectPayload) {
      const project = await createAgentProject(workspaceId, payload);
      await refresh(workspaceId);
      return project;
    },
    async update(workspaceId: string, projectId: string, payload: UpdateAgentProjectPayload) {
      const project = await updateAgentProject(workspaceId, projectId, payload);
      await refresh(workspaceId);
      return project;
    },
    async archive(workspaceId: string, projectId: string) {
      const project = await archiveAgentProject(workspaceId, projectId);
      await refresh(workspaceId);
      return project;
    },
    async restore(workspaceId: string, projectId: string) {
      const project = await restoreAgentProject(workspaceId, projectId);
      await refresh(workspaceId);
      return project;
    },
    async remove(
      workspaceId: string,
      projectId: string,
      deleteContext: boolean,
      discardWorktreeChanges: boolean,
    ) {
      await deleteAgentProject(workspaceId, projectId, deleteContext, discardWorktreeChanges);
      store.getState().removeAgentProject(workspaceId, projectId);
      store.getState().removeAgentProject(workspaceId, projectId, true);
      void refresh(workspaceId);
    },
  };
}

export function projectWorkers(project: AgentProject): AgentProject["tasks"] {
  return project.tasks.filter((task) => task.parent_id === project.main_task_id);
}
