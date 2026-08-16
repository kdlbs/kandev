import type { Repository, Task } from "@/lib/types/http";
import { t } from "@/lib/i18n";

export type RichTaskRowDetails = {
  repositoryLabels: string[];
  description: string | null;
  sessionCount: number | null;
  parentTitle: string | null;
  reviewAttention: "approval_required" | "changes_requested" | null;
};

function reviewAttentionFor(task: Task): RichTaskRowDetails["reviewAttention"] {
  if (task.review_status === "changes_requested") return "changes_requested";
  if (task.review_status === "pending" && task.state !== "IN_PROGRESS") return "approval_required";
  return null;
}

export function resolveRichTaskRowDetails({
  task,
  repositories,
  parentTasks,
}: {
  task: Task;
  repositories: Repository[];
  parentTasks: Task[];
}): RichTaskRowDetails {
  const repositoriesById = new Map(repositories.map((repository) => [repository.id, repository]));
  const seenRepositoryIds = new Set<string>();
  const seenRepositoryLabels = new Set<string>();
  const repositoryLabels: string[] = [];
  for (const taskRepository of [...(task.repositories ?? [])].sort(
    (a, b) => a.position - b.position,
  )) {
    if (seenRepositoryIds.has(taskRepository.repository_id)) continue;
    seenRepositoryIds.add(taskRepository.repository_id);
    const repository = repositoriesById.get(taskRepository.repository_id);
    if (!repository) continue;
    const label = repositoryLabel(repository);
    if (!label) continue;
    if (seenRepositoryLabels.has(label)) continue;
    seenRepositoryLabels.add(label);
    repositoryLabels.push(label);
  }
  const parentTitle = task.parent_id
    ? (parentTasks.find((parentTask) => parentTask.id === task.parent_id)?.title ??
      t("task:subtask"))
    : null;
  const reviewAttention = reviewAttentionFor(task);

  return {
    repositoryLabels,
    description: task.description || null,
    sessionCount: task.session_count && task.session_count > 1 ? task.session_count : null,
    parentTitle,
    reviewAttention,
  };
}

function repositoryLabel(repository: Repository): string | null {
  if (repository.provider_owner && repository.provider_name) {
    return `${repository.provider_owner}/${repository.provider_name}`;
  }
  return repository.name || null;
}
