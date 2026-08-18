import type { StoreApi } from "zustand";
import type { PluginContextApi, TaskCreationContext } from "@kandev/plugin-sdk";
import type { AppState } from "@/lib/state/store";
import { toPluginHostRepository } from "./host-repository";

function taskCreationContext(state: AppState, workspaceId: string): TaskCreationContext | null {
  const workflow =
    state.workflows.items.find(
      (candidate) =>
        candidate.id === state.workflows.activeId && candidate.workspaceId === workspaceId,
    ) ?? state.workflows.items.find((candidate) => candidate.workspaceId === workspaceId);
  if (!workflow) return null;

  const rawSteps =
    state.kanban.workflowId === workflow.id
      ? state.kanban.steps
      : (state.kanbanMulti.snapshots[workflow.id]?.steps ?? []);
  const steps = [...rawSteps]
    .sort((left, right) => left.position - right.position)
    .map((step) => ({
      id: step.id,
      title: step.title,
      ...(step.events ? { events: step.events } : {}),
    }));
  if (!steps[0]) return null;

  return {
    workspaceId,
    workflowId: workflow.id,
    defaultStepId: steps[0].id,
    steps,
    repositories: (state.repositories.itemsByWorkspaceId[workspaceId] ?? []).map(
      toPluginHostRepository,
    ),
  };
}

function subscribeDerived<T>(
  store: StoreApi<AppState>,
  read: (state: AppState) => T,
  fingerprint: (value: T) => string,
  listener: (value: T) => void,
): () => void {
  let previous = fingerprint(read(store.getState()));
  return store.subscribe((state) => {
    const next = read(state);
    const nextFingerprint = fingerprint(next);
    if (nextFingerprint === previous) return;
    previous = nextFingerprint;
    listener(next);
  });
}

function contextFingerprint(context: TaskCreationContext | null): string {
  return JSON.stringify(context);
}

function workspaceIds(state: AppState): string[] {
  return state.workspaces.items.map((workspace) => workspace.id);
}

function workspaceIdsFingerprint(ids: readonly string[]): string {
  return JSON.stringify(ids);
}

export function buildPluginContextApi(store: StoreApi<AppState>): PluginContextApi {
  return {
    getActiveWorkspaceId: () => store.getState().workspaces.activeId ?? undefined,
    subscribeActiveWorkspace: (listener) =>
      subscribeDerived(
        store,
        (state) => state.workspaces.activeId ?? undefined,
        (value) => value ?? "",
        listener,
      ),
    getWorkspaceIds: () => workspaceIds(store.getState()),
    subscribeWorkspaces: (listener) =>
      subscribeDerived(store, workspaceIds, workspaceIdsFingerprint, listener),
    getTaskCreationContext: (workspaceId) => taskCreationContext(store.getState(), workspaceId),
    subscribeTaskCreationContext: (workspaceId, listener) =>
      subscribeDerived(
        store,
        (state) => taskCreationContext(state, workspaceId),
        contextFingerprint,
        listener,
      ),
    resolveRepositoryId: (identity) => {
      const matches = (store.getState().repositories.itemsByWorkspaceId[identity.workspaceId] ?? [])
        .filter(
          (repository) =>
            repository.provider.toLowerCase() === identity.providerId.toLowerCase() &&
            repository.provider_scope === identity.providerScope &&
            repository.provider_repo_id === identity.providerRepositoryId,
        )
        .map((repository) => repository.id);
      return matches.length === 1 ? matches[0] : undefined;
    },
  };
}
