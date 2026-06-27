import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];

type TaskRepositoryFields = Pick<KanbanTask, "repositoryId" | "repositories">;

export function mergeTaskRepositoryFields(
  existing: TaskRepositoryFields | undefined,
  next: TaskRepositoryFields,
): TaskRepositoryFields {
  const repositoriesProvided = next.repositories !== undefined;
  const repositoryIdChanged =
    next.repositoryId !== undefined && next.repositoryId !== existing?.repositoryId;

  return {
    repositoryId: repositoriesProvided
      ? next.repositoryId
      : (next.repositoryId ?? existing?.repositoryId),
    repositories:
      repositoriesProvided || repositoryIdChanged ? next.repositories : existing?.repositories,
  };
}
