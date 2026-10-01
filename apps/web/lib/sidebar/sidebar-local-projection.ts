import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import type { AppState } from "@/lib/state/app-state-types";
import type { TaskOverview } from "@/lib/state/slices/task-overview-types";
import type { SidebarTaskQuery } from "@/lib/types/http";
import { repositoryId } from "@/lib/types/ids";
import { getStateBucket } from "./effective-task-tree-state";
import { matchesSidebarClause } from "./sidebar-local-filter";
import { sqliteBinary } from "./sidebar-local-order";

export type LocalSidebarMetadata = Pick<AppState, "repositories" | "workflows" | "kanbanMulti">;
export type LocalSidebarTask = TaskSwitcherItem & { overview: TaskOverview };

export function sidebarCandidate(task: TaskOverview): boolean {
  const config = task.metadata?.config_mode;
  return task.origin !== "automation_run" && config !== true && config !== 1;
}

export function projectLocalSidebarTasks(
  tasks: TaskOverview[],
  metadata: LocalSidebarMetadata,
  workspaceId: string,
): LocalSidebarTask[] {
  const repos = new Map(
    (metadata.repositories.itemsByWorkspaceId[workspaceId] ?? []).map((repo) => [repo.id, repo]),
  );
  const workflows = new Map(metadata.workflows.items.map((workflow) => [workflow.id, workflow]));
  return tasks.filter(sidebarCandidate).map((task) => {
    const links = [...(task.repositories ?? [])].sort(
      (a, b) => a.position - b.position || sqliteBinary(a.id, b.id),
    );
    const ids = [...new Set(links.map((link) => link.repository_id))];
    const names = ids.flatMap((id) => {
      const repo = repos.get(repositoryId(id));
      return repo
        ? [
            repo.provider_owner && repo.provider_name
              ? `${repo.provider_owner}/${repo.provider_name}`
              : repo.name,
          ]
        : [];
    });
    const summary = task.statusSummary;
    const step = metadata.kanbanMulti.snapshots[task.workflowId]?.steps.find(
      (candidate) => candidate.id === task.workflowStepId,
    );
    return {
      overview: task,
      id: task.id,
      title: task.title,
      state: task.state,
      workspaceId: task.workspaceId,
      workflowId: task.workflowId,
      workflowName: workflows.get(task.workflowId)?.name || "undefined",
      workflowStepId: task.workflowStepId,
      workflowStepTitle: step?.title || "undefined",
      workflowStepColor: step?.color,
      parentTaskId: task.parentTaskId ?? undefined,
      isArchived: task.isArchived,
      sessionState: summary?.primary_session?.state,
      remoteExecutorType: task.primaryExecutorType ?? undefined,
      repositoryLinks: links,
      repositories: names,
      repositoryPath: names[0],
      createdAt: task.createdAt,
      updatedAt: task.updatedAt,
      lastActivityAt: summary?.last_activity_at || task.updatedAt || task.createdAt,
    };
  });
}

export function matchesLocalSidebarTask(
  task: LocalSidebarTask,
  filters: SidebarTaskQuery["filters"],
): boolean {
  if (!filters.some((filter) => filter.dimension === "archived") && task.isArchived) return false;
  const dimensions: Record<string, string | boolean | undefined> = {
    archived: task.isArchived === true,
    state: getStateBucket(task),
    workflow: task.workflowId || undefined,
    workflowStep: task.workflowStepId || undefined,
    executorType: task.remoteExecutorType,
    repository: task.repositoryPath,
    titleMatch: task.title,
    ...sidebarBooleanDimensions(task.overview),
  };
  return filters.every((clause) =>
    matchesSidebarClause(String(dimensions[clause.dimension]), clause),
  );
}

function sidebarBooleanDimensions(overview: TaskOverview) {
  const { git, pull_request: pullRequest } = overview.statusSummary ?? {};
  return {
    hasDiff: (git?.additions ?? 0) > 0 || (git?.deletions ?? 0) > 0,
    hasPR: (pullRequest?.count ?? 0) > 0 || Boolean(pullRequest?.url),
    isPRReview: hasWatchId(overview.metadata?.review_watch_id),
    isIssueWatch: hasWatchId(overview.metadata?.issue_watch_id),
  };
}

function hasWatchId(value: unknown) {
  return Boolean(value && value !== "undefined");
}
