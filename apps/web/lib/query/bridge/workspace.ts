import type { QueryClient } from "@tanstack/react-query";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "../keys";
import { registerBridgeHandlers, type QueryBridgeRegistration } from "./registrar";

type WorkspacePayload = BackendMessageMap["workspace.updated"]["payload"];
type WorkflowPayload = BackendMessageMap["workflow.updated"]["payload"];
type RepositoryPayload = BackendMessageMap["repository.updated"]["payload"];

export function registerWorkspaceBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
): QueryBridgeRegistration {
  return registerBridgeHandlers(ws, queryClient, {
    "workflow.created": (message) => {
      patchWorkflowInCachedLists(queryClient, message.payload);
      invalidateWorkflows(queryClient);
    },
    "workflow.deleted": (message) => {
      queryClient.removeQueries({
        exact: true,
        queryKey: qk.workflows.snapshot(message.payload.id),
      });
      queryClient.removeQueries({ exact: true, queryKey: qk.workflows.steps(message.payload.id) });
      removeWorkflowFromCachedLists(queryClient, message.payload.id);
      invalidateWorkflows(queryClient);
    },
    "workflow.step.created": (message) => {
      invalidateWorkflowSnapshot(queryClient, message.payload.step.workflow_id);
      invalidateWorkflowSteps(queryClient, message.payload.step.workflow_id);
      invalidateWorkflows(queryClient);
    },
    "workflow.step.deleted": (message) => {
      invalidateWorkflowSnapshot(queryClient, message.payload.step.workflow_id);
      invalidateWorkflowSteps(queryClient, message.payload.step.workflow_id);
      invalidateWorkflows(queryClient);
    },
    "workflow.step.updated": (message) => {
      invalidateWorkflowSnapshot(queryClient, message.payload.step.workflow_id);
      invalidateWorkflowSteps(queryClient, message.payload.step.workflow_id);
      invalidateWorkflows(queryClient);
    },
    "workflow.updated": (message) => {
      invalidateWorkflowSnapshot(queryClient, message.payload.id);
      patchWorkflowInCachedLists(queryClient, message.payload);
      invalidateWorkflows(queryClient);
    },
    "repository.created": (message) => {
      patchRepositoryInCachedLists(queryClient, message.payload);
      invalidateRepositoryCaches(queryClient, message.payload);
    },
    "repository.updated": (message) => {
      patchRepositoryInCachedLists(queryClient, message.payload);
      invalidateRepositoryCaches(queryClient, message.payload);
    },
    "repository.deleted": (message) => {
      removeRepositoryFromCachedLists(queryClient, message.payload.id);
      invalidateRepositoryCaches(queryClient, message.payload);
    },
    "repository.script.created": (message) =>
      invalidateRepositoryScriptCaches(queryClient, message.payload),
    "repository.script.updated": (message) =>
      invalidateRepositoryScriptCaches(queryClient, message.payload),
    "repository.script.deleted": (message) =>
      invalidateRepositoryScriptCaches(queryClient, message.payload),
    "workspace.created": () => {
      queryClient.invalidateQueries({ queryKey: qk.workspaces.all() });
    },
    "workspace.deleted": (message) => {
      queryClient.setQueryData(qk.workspaces.all(), (current: unknown) => {
        if (!Array.isArray(current)) return current;
        return current.filter((workspace) => !hasWorkspaceId(workspace, message.payload.id));
      });
      queryClient.invalidateQueries({ queryKey: qk.workspaces.all() });
      invalidateWorkspaceScopedQueries(queryClient, message.payload.id);
    },
    "workspace.updated": (message) => {
      patchWorkspaceList(queryClient, message.payload);
      queryClient.invalidateQueries({ queryKey: qk.workspaces.all() });
    },
  });
}

function patchWorkspaceList(queryClient: QueryClient, payload: WorkspacePayload): void {
  queryClient.setQueryData(qk.workspaces.all(), (current: unknown) => {
    if (!Array.isArray(current)) return current;
    return current.map((workspace) =>
      hasWorkspaceId(workspace, payload.id) && isRecord(workspace)
        ? { ...workspace, ...payload }
        : workspace,
    );
  });
}

function invalidateWorkflows(queryClient: QueryClient): void {
  queryClient.invalidateQueries({ queryKey: ["workflows"] });
}

function removeWorkflowFromCachedLists(queryClient: QueryClient, workflowId: string): void {
  queryClient.setQueriesData(
    { predicate: (query) => isWorkflowListKey(query.queryKey) },
    (current: unknown) => {
      if (!Array.isArray(current)) return current;
      return current.filter((workflow) => !hasWorkflowId(workflow, workflowId));
    },
  );
}

function patchWorkflowInCachedLists(queryClient: QueryClient, payload: WorkflowPayload): void {
  const queries = queryClient.getQueryCache().findAll({
    predicate: (query) => isWorkflowListKey(query.queryKey),
  });
  for (const query of queries) {
    queryClient.setQueryData(query.queryKey, (current: unknown) =>
      patchWorkflowList(query.queryKey, current, payload),
    );
  }
}

function patchWorkflowList(
  queryKey: readonly unknown[],
  current: unknown,
  payload: WorkflowPayload,
): unknown {
  if (!Array.isArray(current) || workflowListWorkspaceId(queryKey) !== payload.workspace_id) {
    return current;
  }
  const includeHidden = workflowListIncludesHidden(queryKey);
  const isVisible = payload.hidden !== true;
  const hasExisting = current.some((workflow) => hasWorkflowId(workflow, payload.id));
  if (!includeHidden && !isVisible) {
    return current.filter((workflow) => !hasWorkflowId(workflow, payload.id));
  }
  if (hasExisting) {
    return current.map((workflow) =>
      hasWorkflowId(workflow, payload.id) && isRecord(workflow)
        ? { ...workflow, ...payload }
        : workflow,
    );
  }
  return includeHidden || isVisible ? [...current, payload] : current;
}

function isWorkflowListKey(queryKey: readonly unknown[]): boolean {
  return (
    queryKey.length === 3 &&
    queryKey[0] === "workflows" &&
    typeof queryKey[1] === "string" &&
    isRecord(queryKey[2]) &&
    "includeHidden" in queryKey[2]
  );
}

function workflowListWorkspaceId(queryKey: readonly unknown[]): string | null {
  return typeof queryKey[1] === "string" ? queryKey[1] : null;
}

function workflowListIncludesHidden(queryKey: readonly unknown[]): boolean {
  return isRecord(queryKey[2]) && queryKey[2].includeHidden === true;
}

function removeRepositoryFromCachedLists(queryClient: QueryClient, repositoryId: string): void {
  queryClient.setQueriesData(
    { predicate: (query) => isWorkspaceRepositoryListKey(query.queryKey) },
    (current: unknown) => {
      if (!Array.isArray(current)) return current;
      return current.filter((repository) => !hasRepositoryId(repository, repositoryId));
    },
  );
}

function patchRepositoryInCachedLists(queryClient: QueryClient, payload: RepositoryPayload): void {
  const queries = queryClient.getQueryCache().findAll({
    predicate: (query) => isWorkspaceRepositoryListKey(query.queryKey),
  });
  for (const query of queries) {
    queryClient.setQueryData(query.queryKey, (current: unknown) =>
      patchRepositoryList(query.queryKey, current, payload),
    );
  }
}

function patchRepositoryList(
  queryKey: readonly unknown[],
  current: unknown,
  payload: RepositoryPayload,
): unknown {
  if (!Array.isArray(current)) return current;
  const workspaceId = workspaceRepositoryListWorkspaceId(queryKey);
  if (payload.workspace_id && workspaceId !== payload.workspace_id) return current;
  const hasExisting = current.some((repository) => hasRepositoryId(repository, payload.id));
  if (hasExisting) {
    return current.map((repository) =>
      hasRepositoryId(repository, payload.id) && isRecord(repository)
        ? { ...repository, ...payload }
        : repository,
    );
  }
  return payload.workspace_id === workspaceId ? [...current, payload] : current;
}

function workspaceRepositoryListWorkspaceId(queryKey: readonly unknown[]): string | null {
  return typeof queryKey[1] === "string" ? queryKey[1] : null;
}

function invalidateWorkflowSnapshot(queryClient: QueryClient, workflowId: string): void {
  queryClient.invalidateQueries({ exact: true, queryKey: qk.workflows.snapshot(workflowId) });
}

function invalidateWorkflowSteps(queryClient: QueryClient, workflowId: string): void {
  queryClient.invalidateQueries({ exact: true, queryKey: qk.workflows.steps(workflowId) });
}

function invalidateRepositoryCaches(
  queryClient: QueryClient,
  payload: BackendMessageMap["repository.updated"]["payload"],
): void {
  if (typeof payload.workspace_id === "string" && payload.workspace_id) {
    queryClient.invalidateQueries({
      queryKey: ["workspaces", payload.workspace_id, "repositories"],
    });
  } else {
    invalidateAllWorkspaceRepositoryLists(queryClient);
  }
  if (typeof payload.id === "string" && payload.id) {
    queryClient.invalidateQueries({
      exact: true,
      queryKey: qk.workspaces.repositoryScripts(payload.id),
    });
  }
}

function invalidateRepositoryScriptCaches(
  queryClient: QueryClient,
  payload: BackendMessageMap["repository.script.updated"]["payload"],
): void {
  if (typeof payload.repository_id === "string" && payload.repository_id) {
    queryClient.invalidateQueries({
      exact: true,
      queryKey: qk.workspaces.repositoryScripts(payload.repository_id),
    });
  }
  invalidateAllWorkspaceRepositoryLists(queryClient);
}

function invalidateAllWorkspaceRepositoryLists(queryClient: QueryClient): void {
  queryClient.invalidateQueries({
    predicate: (query) => isWorkspaceRepositoryListKey(query.queryKey),
  });
}

function isWorkspaceRepositoryListKey(queryKey: readonly unknown[]): boolean {
  return (
    queryKey.length === 4 &&
    queryKey[0] === "workspaces" &&
    typeof queryKey[1] === "string" &&
    queryKey[2] === "repositories" &&
    isRecord(queryKey[3]) &&
    "includeScripts" in queryKey[3]
  );
}

function invalidateWorkspaceScopedQueries(queryClient: QueryClient, workspaceId: string): void {
  // Workspace switches update the active workspace before workspace-scoped
  // queries enable, so broad stale marking here is enough; enabled guards avoid
  // refetching the previous workspace after deletion.
  queryClient.invalidateQueries({ queryKey: ["workflows", workspaceId] });
  queryClient.invalidateQueries({ queryKey: ["workspaces", workspaceId] });
  queryClient.invalidateQueries({ queryKey: ["tasks", "page", workspaceId] });
  queryClient.invalidateQueries({ queryKey: ["tasks", "infinite", workspaceId] });
}

function hasWorkspaceId(value: unknown, id: string): boolean {
  return isRecord(value) && value.id === id;
}

function hasWorkflowId(value: unknown, id: string): boolean {
  return isRecord(value) && value.id === id;
}

function hasRepositoryId(value: unknown, id: string): boolean {
  return isRecord(value) && value.id === id;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
