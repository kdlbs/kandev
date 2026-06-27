import type { QueryClient } from "@tanstack/react-query";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "../keys";
import { registerBridgeHandlers, type QueryBridgeRegistration } from "./registrar";

type WorkspacePayload = BackendMessageMap["workspace.updated"]["payload"];

export function registerWorkspaceBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
): QueryBridgeRegistration {
  return registerBridgeHandlers(ws, queryClient, {
    "workflow.created": () => invalidateWorkflows(queryClient),
    "workflow.deleted": (message) => {
      queryClient.removeQueries({
        exact: true,
        queryKey: qk.workflows.snapshot(message.payload.id),
      });
      queryClient.removeQueries({ exact: true, queryKey: qk.workflows.steps(message.payload.id) });
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
      invalidateWorkflows(queryClient);
    },
    "repository.created": (message) => invalidateRepositoryCaches(queryClient, message.payload),
    "repository.updated": (message) => invalidateRepositoryCaches(queryClient, message.payload),
    "repository.deleted": (message) => invalidateRepositoryCaches(queryClient, message.payload),
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
  return queryKey[0] === "workspaces" && queryKey[2] === "repositories";
}

function invalidateWorkspaceScopedQueries(queryClient: QueryClient, workspaceId: string): void {
  queryClient.invalidateQueries({ queryKey: ["workflows", workspaceId] });
  queryClient.invalidateQueries({ queryKey: ["workspaces", workspaceId] });
  queryClient.invalidateQueries({ queryKey: ["tasks", "page", workspaceId] });
  queryClient.invalidateQueries({ queryKey: ["tasks", "infinite", workspaceId] });
}

function hasWorkspaceId(value: unknown, id: string): boolean {
  return isRecord(value) && value.id === id;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
