import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];

type TaskRepositoryFields = Pick<KanbanTask, "repositoryId" | "repositories">;

export function mergeTaskRepositoryFields(
  existing: TaskRepositoryFields | undefined,
  next: TaskRepositoryFields,
): TaskRepositoryFields {
  const repositoriesProvided = next.repositories !== undefined;
  const nextRepositoryId = next.repositoryId ?? next.repositories?.[0]?.repository_id;
  const repositoryIdChanged =
    nextRepositoryId !== undefined && nextRepositoryId !== existing?.repositoryId;

  return {
    repositoryId: repositoriesProvided
      ? nextRepositoryId
      : (nextRepositoryId ?? existing?.repositoryId),
    repositories:
      repositoriesProvided || repositoryIdChanged ? next.repositories : existing?.repositories,
  };
}
