import type { AppState } from "../app-state-types";
import type { SidebarView } from "./ui/sidebar-view-types";
import type { TaskOverview } from "./task-overview-types";
import {
  matchesSidebarClause,
  sidebarCanIncludeArchives,
} from "@/lib/sidebar/sidebar-local-filter";

export type SidebarInventory = Pick<
  AppState,
  | "workspaces"
  | "workflows"
  | "kanbanMulti"
  | "workspaceContextRead"
  | "workspaceContextGeneration"
  | "auth"
  | "repositories"
>;

function currentContext(state: SidebarInventory, workspaceId: string | null) {
  if (!workspaceId || state.workspaces.activeId !== workspaceId) return false;
  if (state.auth.mode !== "disabled" && !state.auth.authenticated) return false;
  const read = state.workspaceContextRead;
  return (
    read.workspaceId === workspaceId &&
    read.generation === state.workspaceContextGeneration &&
    read.snapshotError !== "access_denied" &&
    !Object.values(read.errors).includes("access_denied")
  );
}

function workflowScope(
  state: SidebarInventory,
  workspaceId: string,
  view: SidebarView,
): string[] | null {
  const clauses = view.filters.filter((filter) => filter.dimension === "workflow");
  const positive = clauses.find((filter) => filter.op === "is" || filter.op === "in");
  if (positive) {
    const ids = Array.isArray(positive.value) ? positive.value : [String(positive.value)];
    return [...new Set(ids)].filter((id) =>
      clauses.every((clause) => matchesSidebarClause(id, clause)),
    );
  }
  const coverage = state.workflows.taskWorkflowCoverage;
  if (!coverage?.complete || coverage.workspace_id !== workspaceId) return null;
  return coverage.workflow_ids.filter((id) =>
    clauses.every((clause) => matchesSidebarClause(id || "undefined", clause)),
  );
}

function requiredFields(view: SidebarView): Array<keyof TaskOverview> {
  const fields: Array<keyof TaskOverview> = [
    "id",
    "title",
    "workflowId",
    "workflowStepId",
    "parentTaskId",
    "updatedAt",
    "createdAt",
    "metadata",
    "origin",
  ];
  const dimensions = new Set(view.filters.map((filter) => filter.dimension));
  if (view.group === "state" || view.sort.key === "state" || dimensions.has("state"))
    fields.push("state", "statusSummary");
  if (view.sort.key === "lastActivityAt" || dimensions.has("hasPR") || dimensions.has("hasDiff"))
    fields.push("statusSummary");
  if (view.group === "repository" || dimensions.has("repository")) fields.push("repositories");
  if (view.group === "executorType" || dimensions.has("executorType"))
    fields.push("primaryExecutorType");
  return fields;
}

function completeSnapshot(
  snapshot: SidebarInventory["kanbanMulti"]["snapshots"][string] | undefined,
  workspaceId: string,
  workflowId: string,
) {
  if (!snapshot) return false;
  const coverage = snapshot.taskCoverage;
  return (
    coverage?.complete &&
    !snapshot.isPlaceholder &&
    !snapshot.fetchFailed &&
    coverage.workspace_id === workspaceId &&
    coverage.workflow_id === workflowId &&
    coverage.membership === "active" &&
    coverage.ordering_profile === "sqlite_nocase_v1" &&
    coverage.total === snapshot.tasks.length
  );
}

function coveredTask(
  task: TaskOverview,
  workspaceId: string,
  workflowId: string,
  fields: Array<keyof TaskOverview>,
) {
  return (
    task.workflowId === workflowId &&
    (!task.workspaceId || task.workspaceId === workspaceId) &&
    !task.isArchived &&
    fields.every((field) => Object.hasOwn(task, field)) &&
    typeof task.createdAt === "string" &&
    typeof task.updatedAt === "string"
  );
}

/** A record count never substitutes for authoritative, current scope coverage. */
export function coveredTaskOverviews(
  state: SidebarInventory,
  workspaceId: string | null,
  view: SidebarView,
): TaskOverview[] | null {
  if (!currentContext(state, workspaceId) || sidebarCanIncludeArchives(view.filters)) return null;
  const workflows = workflowScope(state, workspaceId!, view);
  if (!workflows) return null;
  const fields = requiredFields(view);
  const needsRepositories =
    view.group === "repository" || view.filters.some((filter) => filter.dimension === "repository");
  if (needsRepositories && !Object.hasOwn(state.repositories.itemsByWorkspaceId, workspaceId!))
    return null;
  const tasks: TaskOverview[] = [];
  for (const id of workflows) {
    const snapshot = state.kanbanMulti.snapshots[id];
    if (!completeSnapshot(snapshot, workspaceId!, id)) return null;
    for (const task of snapshot.tasks) {
      if (!coveredTask(task, workspaceId!, id, fields)) return null;
      tasks.push(task);
    }
  }
  return tasks;
}
