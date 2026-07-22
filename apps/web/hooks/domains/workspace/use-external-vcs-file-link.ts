"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Repository, Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";
import type { ExternalVcsFileURL } from "@/lib/utils/external-vcs-file-url";
import { resolveExternalVcsFileURL } from "@/lib/utils/external-vcs-file-url";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useAzureDevOpsTaskPullRequests } from "@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests";

export type UseExternalVcsFileLinkInput = {
  filePath: string;
  previousPath?: string | null;
  status?: string | null;
  taskId?: string | null;
  sessionId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string | null;
  publishedBranch?: string | null;
  publishedPullRequestNumber?: number | null;
  baseBranch?: string | null;
};

type TaskRepositoryLink = NonNullable<KanbanState["tasks"][number]["repositories"]>[number];

const EMPTY_TASK_REPOSITORIES: TaskRepositoryLink[] = [];
const EMPTY_GITHUB_PRS: TaskPR[] = [];
const EMPTY_GITLAB_MRS: TaskMR[] = [];
const EMPTY_AZURE_PRS: AzureDevOpsTaskPullRequest[] = [];

type LinkSnapshot = {
  repositories: Repository[];
  taskRepositories: TaskRepositoryLink[];
  sessionRepositoryId?: string;
  sessionWorktreeBranch?: string;
  sessionWorktrees: Worktree[];
  githubPRs: TaskPR[];
  gitlabMRs: TaskMR[];
  azurePRs: AzureDevOpsTaskPullRequest[];
};

function sanitizedRepositoryName(value: string): string {
  return value
    .replace(/[^A-Za-z0-9_.-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^[-.]+|[-.]+$/g, "");
}

function repositoryMatchesName(repository: Repository, requestedName: string): boolean {
  const requested = sanitizedRepositoryName(requestedName);
  return [repository.name, repository.provider_name].some(
    (name) => name === requestedName || sanitizedRepositoryName(name) === requested,
  );
}

function linkedRepositoryIds(taskRepositories: TaskRepositoryLink[]): Set<string> {
  return new Set(taskRepositories.map((link) => link.repository_id));
}

function linkedRepositoryById(
  repositoryId: string | undefined,
  snapshot: LinkSnapshot,
  linkedIds: Set<string>,
): Repository | null {
  return (
    snapshot.repositories.find(
      (repository) => repository.id === repositoryId && linkedIds.has(repository.id),
    ) ?? null
  );
}

function resolveNamedRepository(
  repositoryName: string,
  snapshot: LinkSnapshot,
  linkedIds: Set<string>,
): Repository | null {
  const namedWorktrees = snapshot.sessionWorktrees.filter(
    (worktree) => basename(worktree.path) === repositoryName,
  );
  if (namedWorktrees.length > 0) {
    return namedWorktrees.length === 1
      ? linkedRepositoryById(namedWorktrees[0].repositoryId, snapshot, linkedIds)
      : null;
  }
  const matches = snapshot.repositories.filter(
    (repository) =>
      linkedIds.has(repository.id) && repositoryMatchesName(repository, repositoryName),
  );
  return matches.length === 1 ? matches[0] : null;
}

function resolveRepository(
  input: UseExternalVcsFileLinkInput,
  snapshot: LinkSnapshot,
): Repository | null {
  const linkedIds = linkedRepositoryIds(snapshot.taskRepositories);
  if (input.repositoryId) {
    return linkedRepositoryById(input.repositoryId, snapshot, linkedIds);
  }
  if (input.repositoryName) {
    return resolveNamedRepository(input.repositoryName, snapshot, linkedIds);
  }

  const worktreeRepositoryIds = new Set(
    snapshot.sessionWorktrees.map((worktree) => worktree.repositoryId).filter(Boolean),
  );
  if (worktreeRepositoryIds.size === 1) {
    const id = Array.from(worktreeRepositoryIds)[0];
    return linkedRepositoryById(id, snapshot, linkedIds);
  }
  if (worktreeRepositoryIds.size === 0 && snapshot.sessionRepositoryId) {
    return linkedRepositoryById(snapshot.sessionRepositoryId, snapshot, linkedIds);
  }
  if (worktreeRepositoryIds.size === 0 && !snapshot.sessionRepositoryId && linkedIds.size === 1) {
    const id = Array.from(linkedIds)[0];
    return snapshot.repositories.find((repository) => repository.id === id) ?? null;
  }
  return null;
}

function basename(path: string | undefined): string {
  return (
    path
      ?.replace(/[\\/]+$/, "")
      .split(/[\\/]/)
      .pop() ?? ""
  );
}

function resolveActiveBranch(
  input: UseExternalVcsFileLinkInput,
  repository: Repository,
  snapshot: LinkSnapshot,
): string | null {
  const repositoryWorktrees = snapshot.sessionWorktrees.filter(
    (worktree) => worktree.repositoryId === repository.id,
  );
  if (input.repositoryName) {
    const named = repositoryWorktrees.filter(
      (worktree) => basename(worktree.path) === input.repositoryName,
    );
    if (named.length === 1) return named[0].branch ?? null;
  }
  if (repositoryWorktrees.length === 1) return repositoryWorktrees[0].branch ?? null;
  if (
    repositoryWorktrees.length === 0 &&
    snapshot.sessionRepositoryId === repository.id &&
    snapshot.sessionWorktreeBranch
  ) {
    return snapshot.sessionWorktreeBranch;
  }
  return null;
}

function resolveTaskRepositoryLink(
  repository: Repository,
  activeBranch: string | null,
  snapshot: LinkSnapshot,
): TaskRepositoryLink | null {
  const links = snapshot.taskRepositories.filter((link) => link.repository_id === repository.id);
  if (links.length === 1) return links[0];
  if (!activeBranch) return null;
  const matches = links.filter(
    (link) => (link.checkout_branch || link.base_branch) === activeBranch,
  );
  return matches.length === 1 ? matches[0] : null;
}

function normalizeOrigin(value: string | undefined): string {
  try {
    return new URL(value ?? "").origin.toLowerCase();
  } catch {
    return "";
  }
}

function githubPRMatches(pr: TaskPR, repository: Repository): boolean {
  if (pr.repository_id) return pr.repository_id === repository.id;
  return (
    repository.provider === "github" &&
    pr.owner === repository.provider_owner &&
    pr.repo === repository.provider_name
  );
}

function gitlabMRMatches(mr: TaskMR, repository: Repository): boolean {
  if (mr.repository_id) return mr.repository_id === repository.id;
  return (
    repository.provider === "gitlab" &&
    normalizeOrigin(mr.host) === normalizeOrigin(repository.provider_host) &&
    mr.project_path === `${repository.provider_owner}/${repository.provider_name}`
  );
}

function publishedBranches(repository: Repository, snapshot: LinkSnapshot): string[] {
  if (repository.provider === "github") {
    return snapshot.githubPRs
      .filter((pr) => githubPRMatches(pr, repository))
      .map((pr) => pr.head_branch)
      .filter(Boolean);
  }
  if (repository.provider === "gitlab") {
    return snapshot.gitlabMRs
      .filter((mr) => gitlabMRMatches(mr, repository))
      .map((mr) => mr.head_branch)
      .filter(Boolean);
  }
  if (repository.provider === "azure_devops") {
    return snapshot.azurePRs
      .filter((pr) => pr.repositoryId === repository.id)
      .map((pr) => pr.sourceBranch)
      .filter(Boolean);
  }
  return [];
}

function resolvePublishedBranch(
  input: UseExternalVcsFileLinkInput,
  repository: Repository,
  activeBranch: string | null,
  snapshot: LinkSnapshot,
): string | null {
  if (input.publishedBranch) return input.publishedBranch;
  const branches = Array.from(new Set(publishedBranches(repository, snapshot)));
  if (activeBranch && branches.includes(activeBranch)) return activeBranch;
  const repositoryLinkCount = snapshot.taskRepositories.filter(
    (link) => link.repository_id === repository.id,
  ).length;
  if (repositoryLinkCount > 1) return null;
  return branches.length === 1 ? branches[0] : null;
}

function resolvePublishedRevision(
  input: UseExternalVcsFileLinkInput,
  repository: Repository,
  activeBranch: string | null,
  snapshot: LinkSnapshot,
): { branch: string; githubPullRequestNumber?: number } | null {
  const branch = resolvePublishedBranch(input, repository, activeBranch, snapshot);
  if (!branch) return null;
  if (input.publishedBranch) {
    return { branch, githubPullRequestNumber: input.publishedPullRequestNumber ?? undefined };
  }
  if (repository.provider !== "github") return { branch };
  const matchingPRs = snapshot.githubPRs.filter(
    (pr) => githubPRMatches(pr, repository) && pr.head_branch === branch,
  );
  return {
    branch,
    githubPullRequestNumber: matchingPRs.length === 1 ? matchingPRs[0].pr_number : undefined,
  };
}

function resolveLink(
  input: UseExternalVcsFileLinkInput,
  snapshot: LinkSnapshot,
): ExternalVcsFileURL | null {
  const repository = resolveRepository(input, snapshot);
  if (!repository) return null;
  const activeBranch = resolveActiveBranch(input, repository, snapshot);
  const taskRepository = resolveTaskRepositoryLink(repository, activeBranch, snapshot);
  if (!taskRepository) return null;
  const publishedRevision = resolvePublishedRevision(input, repository, activeBranch, snapshot);
  return resolveExternalVcsFileURL({
    repository,
    path: input.filePath,
    previousPath: input.previousPath,
    status: input.status,
    publishedBranch: publishedRevision?.branch,
    publishedPullRequestNumber: publishedRevision?.githubPullRequestNumber,
    baseBranch: input.baseBranch || taskRepository.base_branch,
  });
}

export function useExternalVcsFileLink(
  input: UseExternalVcsFileLinkInput,
): ExternalVcsFileURL | null {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const session = useAppStore((state) =>
    input.sessionId ? state.taskSessions.items[input.sessionId] : undefined,
  );
  const resolvedTaskId = input.taskId ?? session?.task_id ?? activeTaskId;
  const taskRepositories = useAppStore(
    (state) =>
      state.kanban.tasks.find((task) => task.id === resolvedTaskId)?.repositories ??
      EMPTY_TASK_REPOSITORIES,
  );
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const worktrees = useAppStore((state) => state.worktrees.items);
  const sessionWorktreeIds = useAppStore((state) =>
    input.sessionId
      ? state.sessionWorktreesBySessionId.itemsBySessionId[input.sessionId]
      : undefined,
  );
  const githubPRs = useAppStore((state) =>
    resolvedTaskId
      ? (state.taskPRs.byTaskId[resolvedTaskId] ?? EMPTY_GITHUB_PRS)
      : EMPTY_GITHUB_PRS,
  );
  const taskMRsByWorkspace = useAppStore((state) => state.taskMRs.byWorkspaceId);
  const azurePRs = useAppStore((state) =>
    resolvedTaskId
      ? (state.azureDevOpsTaskPullRequests.byTaskId[resolvedTaskId] ?? EMPTY_AZURE_PRS)
      : EMPTY_AZURE_PRS,
  );

  const apiWorktrees: Worktree[] = session
    ? (session.worktrees ?? []).map((worktree) => ({
        id: worktree.worktree_id || worktree.id,
        sessionId: session.id,
        repositoryId: worktree.repository_id,
        path: worktree.worktree_path,
        branch: worktree.worktree_branch,
      }))
    : [];
  const seen = new Set(apiWorktrees.map((worktree) => worktree.id));
  const liveWorktrees = (sessionWorktreeIds ?? [])
    .map((id) => worktrees[id])
    .filter((worktree): worktree is Worktree => Boolean(worktree) && !seen.has(worktree.id));
  const gitlabMRs = resolvedTaskId
    ? Object.values(taskMRsByWorkspace).flatMap(
        (taskMRs) => taskMRs[resolvedTaskId] ?? EMPTY_GITLAB_MRS,
      )
    : EMPTY_GITLAB_MRS;
  return resolveLink(input, {
    repositories: Object.values(repositoriesByWorkspace).flat(),
    taskRepositories,
    sessionRepositoryId: session?.repository_id,
    sessionWorktreeBranch: session?.worktree_branch,
    sessionWorktrees: apiWorktrees.concat(liveWorktrees),
    githubPRs,
    gitlabMRs,
    azurePRs,
  });
}

export function useExternalVcsFileLinkHydration(
  task: Pick<Task, "id" | "workspace_id" | "repositories"> | null,
  repositories: Repository[],
): void {
  const providers = useMemo(() => {
    const repositoryIds = new Set(task?.repositories?.map((link) => link.repository_id) ?? []);
    return new Set(
      repositories
        .filter((repository) => repositoryIds.has(repository.id))
        .map((repository) => repository.provider),
    );
  }, [repositories, task?.repositories]);
  const taskId = task?.id ?? null;
  const workspaceId = task?.workspace_id ?? null;
  useTaskPR(providers.has("github") ? taskId : null);
  useWorkspaceMRs(providers.has("gitlab") ? workspaceId : null);
  useAzureDevOpsTaskPullRequests(
    providers.has("azure_devops") ? workspaceId : null,
    providers.has("azure_devops") ? taskId : null,
  );
}
