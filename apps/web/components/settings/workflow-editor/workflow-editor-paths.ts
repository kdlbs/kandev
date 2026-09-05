import type { WorkflowLifecycleTrigger } from "@/lib/workflows/workflow-action-catalog";

export type WorkflowEditorTab = "agent" | "automation" | "policies";

export type WorkflowEditorRouteSelection = {
  stepId: string | null;
  tab: WorkflowEditorTab;
  trigger: WorkflowLifecycleTrigger | null;
  actionIndex: number | null;
};

export function workflowsPath(workspaceId: string): string {
  return `/settings/workspaces/${encodeURIComponent(workspaceId)}/workflows`;
}

export function workflowEditorPath(workspaceId: string, workflowId: string): string {
  return `${workflowsPath(workspaceId)}/${encodeURIComponent(workflowId)}`;
}

export function newWorkflowEditorPath(
  workspaceId: string,
  options: { name?: string; templateId?: string | null } = {},
): string {
  const params = new URLSearchParams();
  if (options.templateId) params.set("template", options.templateId);
  if (options.name?.trim()) params.set("name", options.name.trim());
  const query = params.toString();
  return `${workflowEditorPath(workspaceId, "new")}${query ? `?${query}` : ""}`;
}

export function readWorkflowEditorSelection(params: URLSearchParams): WorkflowEditorRouteSelection {
  const actionIndex = parseActionIndex(params.get("action"));
  const trigger = parseTrigger(params.get("trigger"));
  return {
    stepId: params.get("step"),
    tab: parseTab(params.get("tab")),
    trigger: actionIndex === null ? null : trigger,
    actionIndex: actionIndex === null || trigger === null ? null : actionIndex,
  };
}

export function workflowEditorSelectionPath(
  pathname: string,
  currentParams: URLSearchParams,
  selection: WorkflowEditorRouteSelection,
): string {
  const params = new URLSearchParams(currentParams);
  if (selection.stepId) params.set("step", selection.stepId);
  else params.delete("step");
  if (selection.tab === "agent") params.delete("tab");
  else params.set("tab", selection.tab);
  if (selection.trigger !== null && selection.actionIndex !== null) {
    params.set("trigger", selection.trigger);
    params.set("action", String(selection.actionIndex));
  } else {
    params.delete("trigger");
    params.delete("action");
  }
  const query = params.toString();
  return query ? `${pathname}?${query}` : pathname;
}

function parseTab(value: string | null): WorkflowEditorTab {
  return value === "automation" || value === "policies" ? value : "agent";
}

function parseTrigger(value: string | null): WorkflowLifecycleTrigger | null {
  if (
    value === "on_enter" ||
    value === "on_turn_start" ||
    value === "on_turn_complete" ||
    value === "on_exit"
  ) {
    return value;
  }
  return null;
}

function parseActionIndex(value: string | null): number | null {
  if (!value || !/^\d+$/.test(value)) return null;
  const index = Number(value);
  return Number.isSafeInteger(index) ? index : null;
}
