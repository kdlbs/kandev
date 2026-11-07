export type TasksListArchiveMode = "active" | "archived" | "all";
export type TasksListScope = { workflowId: string | null; repositoryId: string | null };
export type TasksListMetadataReady = { workflows: boolean; repositories: boolean };
export type TasksListQuery = TasksListScope & {
  archiveMode: TasksListArchiveMode;
  searchQuery: string;
};

export function resolveTasksListQuery(
  params: URLSearchParams,
  defaults: TasksListScope,
): TasksListQuery {
  const archive = params.get("archive");
  return {
    archiveMode: archive === "archived" || archive === "all" ? archive : "active",
    searchQuery: params.get("q") ?? "",
    workflowId: resolveScope(params.get("workflow"), defaults.workflowId),
    repositoryId: resolveScope(params.get("repository"), defaults.repositoryId),
  };
}

function resolveScope(value: string | null, fallback: string | null): string | null {
  if (value === null) return fallback;
  return value === "all" ? null : value;
}

export function tasksListArchiveParams(mode: TasksListArchiveMode): {
  includeArchived?: boolean;
  onlyArchived?: boolean;
} {
  if (mode === "archived") return { onlyArchived: true };
  return mode === "all" ? { includeArchived: true } : {};
}

export function canUseTasksListBootData(
  params: URLSearchParams,
  bootWorkspaceId: string | null | undefined,
  workspaceId: string | null | undefined,
) {
  return (
    Boolean(workspaceId && bootWorkspaceId === workspaceId) &&
    !["archive", "q", "workflow", "repository"].some((field) => params.has(field))
  );
}

export function tasksListFilterStatus(input: {
  query: TasksListScope;
  workflowIds: readonly string[];
  repositoryIds: readonly string[];
  workflowReady: boolean;
  repositoryReady: boolean;
  workflowFailed: boolean;
  repositoryFailed: boolean;
}) {
  const workflow = filterFieldStatus(
    input.query.workflowId,
    input.workflowIds,
    input.workflowReady,
    input.workflowFailed,
  );
  const repository = filterFieldStatus(
    input.query.repositoryId,
    input.repositoryIds,
    input.repositoryReady,
    input.repositoryFailed,
  );
  const metadataError = workflow.failed || repository.failed;
  let unavailableFilter: "workflow" | "repository" | null = null;
  if (!metadataError && workflow.missing) unavailableFilter = "workflow";
  if (!metadataError && !unavailableFilter && repository.missing) unavailableFilter = "repository";
  return {
    unavailableFilter,
    metadataError,
    metadataLoading: !metadataError && (workflow.loading || repository.loading),
  };
}

function filterFieldStatus(
  id: string | null,
  ids: readonly string[],
  ready: boolean,
  failed: boolean,
) {
  return {
    loading: id !== null && !ready && !failed,
    failed: id !== null && failed,
    missing: id !== null && ready && !failed && !ids.includes(id),
  };
}
