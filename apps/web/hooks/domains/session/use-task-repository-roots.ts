"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { TaskSessionWorktree } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

export type TaskRepositoryRoot = {
  key: string;
  taskId: string;
  sessionId: string;
  taskRepositoryId: string;
  repositoryId: string;
  repositoryName: string;
  trackerName: string;
  path: string;
  relativePath: string;
  branch: string;
  worktreeId: string;
  environmentId: string;
  isPrimary: boolean;
  removable: boolean;
  adopted: boolean;
  displayLabel: string;
};

export type TaskRepositoryRoots = {
  roots: TaskRepositoryRoot[];
  byTrackerName: Map<string, TaskRepositoryRoot>;
  byPath: Map<string, TaskRepositoryRoot>;
};

type RootRepository = NonNullable<
  NonNullable<AppState["kanban"]["tasks"][number]>["repositories"]
>[number];

function repoBasename(path: string): string {
  const segments = path.replace(/\\/g, "/").split("/");
  return segments.filter(Boolean).pop() ?? "";
}

function relativeToWorkspace(path: string, workspacePath: string): string {
  if (!path || !workspacePath) return "";
  const normalizedPath = path.replace(/\\/g, "/").replace(/\/+$/, "");
  const normalizedRoot = workspacePath.replace(/\\/g, "/").replace(/\/+$/, "");
  if (normalizedPath === normalizedRoot) return "";
  if (normalizedPath.startsWith(`${normalizedRoot}/`)) {
    return normalizedPath.slice(normalizedRoot.length + 1);
  }
  return "";
}

function findWorktree(
  worktrees: TaskSessionWorktree[],
  repository: RootRepository,
): TaskSessionWorktree | undefined {
  const branch = repository.checkout_branch || repository.base_branch || "";
  const relativePath = repository.workspace_relative_path || "";
  return worktrees.find((worktree) => {
    if (worktree.workspace_relative_path && relativePath) {
      return worktree.workspace_relative_path === relativePath;
    }
    if (worktree.repository_id === repository.repository_id && worktree.worktree_branch && branch) {
      return worktree.worktree_branch === branch;
    }
    return (
      worktree.repository_id === repository.repository_id &&
      worktree.position === repository.position &&
      !worktree.worktree_branch
    );
  });
}

function resolveRootPath(
  worktreePath: string | undefined,
  relativePath: string,
  workspacePath: string,
  isPrimary: boolean,
): string {
  if (worktreePath) return worktreePath;
  if (relativePath && workspacePath) {
    return `${workspacePath.replace(/\/+$/, "")}/${relativePath.replace(/^\/+/, "")}`;
  }
  return isPrimary ? workspacePath : "";
}

function rootBranch(repository: RootRepository, worktree?: TaskSessionWorktree): string {
  return worktree?.worktree_branch || repository.checkout_branch || repository.base_branch || "";
}

function rootRelativePath(
  repository: RootRepository,
  worktree: TaskSessionWorktree | undefined,
  path: string,
  workspacePath: string,
): string {
  return (
    worktree?.workspace_relative_path ||
    repository.workspace_relative_path ||
    relativeToWorkspace(path, workspacePath)
  );
}

function rootDisplayLabel(repositoryName: string, branch: string): string {
  return branch ? `${repositoryName} · ${branch}` : repositoryName;
}

type SessionProjection = {
  taskId: string;
  sessionId: string;
  workspacePath: string;
  environmentId: string;
  worktrees: TaskSessionWorktree[];
};

function projectSession(
  session: AppState["taskSessions"]["items"][string] | null,
): SessionProjection {
  if (!session) {
    return { taskId: "", sessionId: "", workspacePath: "", environmentId: "", worktrees: [] };
  }
  return {
    taskId: session.task_id,
    sessionId: session.id,
    workspacePath: session.workspace_path ?? "",
    environmentId: session.environment_id ?? "",
    worktrees: session.worktrees ?? [],
  };
}

function resolveRepositoryName(
  repositoryNames: Map<string, string>,
  repository: RootRepository,
  path: string,
): string {
  const known = repositoryNames.get(repository.repository_id);
  if (known) return known;
  return repoBasename(path);
}

function createRoot(
  repository: RootRepository,
  index: number,
  session: SessionProjection,
  repositoryNames: Map<string, string>,
): TaskRepositoryRoot | null {
  const worktree = findWorktree(session.worktrees, repository);
  const isPrimary = index === 0;
  const path = resolveRootPath(
    worktree?.worktree_path,
    repository.workspace_relative_path ?? "",
    session.workspacePath,
    isPrimary,
  );
  if (!path) return null;
  const repositoryName = resolveRepositoryName(repositoryNames, repository, path);
  const branch = rootBranch(repository, worktree);
  const resolvedRelative = rootRelativePath(repository, worktree, path, session.workspacePath);
  const trackerName =
    worktree?.worktree_origin === "adopted" && resolvedRelative
      ? resolvedRelative
      : repoBasename(path);
  return {
    key: repository.id,
    taskId: session.taskId,
    sessionId: session.sessionId,
    taskRepositoryId: repository.id,
    repositoryId: repository.repository_id,
    repositoryName,
    trackerName,
    path,
    relativePath: resolvedRelative,
    branch,
    worktreeId: worktree?.worktree_id ?? "",
    environmentId: session.environmentId,
    isPrimary,
    removable: !isPrimary,
    adopted: worktree?.worktree_origin === "adopted",
    displayLabel: rootDisplayLabel(repositoryName, branch),
  };
}

function buildRoots(
  repositories: RootRepository[],
  session: AppState["taskSessions"]["items"][string] | null,
  repositoryNames: Map<string, string>,
): TaskRepositoryRoots {
  const projected = projectSession(session);
  const roots = [...repositories]
    .sort((left, right) => left.position - right.position)
    .map((repository, index) => createRoot(repository, index, projected, repositoryNames))
    .filter((root): root is TaskRepositoryRoot => root !== null);
  const byTrackerName = new Map<string, TaskRepositoryRoot>();
  const byPath = new Map<string, TaskRepositoryRoot>();
  for (const root of roots) {
    if (root.relativePath) byPath.set(root.relativePath, root);
    if (root.trackerName) byTrackerName.set(root.trackerName, root);
    if (root.isPrimary) byTrackerName.set("", root);
  }
  return { roots, byTrackerName, byPath };
}

function useTaskRepositoryRootsFromIds(
  taskId: string | null,
  sessionId: string | null,
): TaskRepositoryRoots {
  const repositories = useAppStore((state) =>
    taskId
      ? ((state.kanban?.tasks ?? []).find((task) => task.id === taskId)?.repositories ?? [])
      : [],
  );
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions?.items?.[sessionId] ?? null) : null,
  );
  const workspaceId = useAppStore(
    (state) => state.kanban?.tasks?.find((task) => task.id === taskId)?.workspaceId ?? undefined,
  );
  const repositoryEntities = useAppStore((state) =>
    workspaceId ? state.repositories?.itemsByWorkspaceId?.[workspaceId] : undefined,
  );
  const repositoryNames = useMemo(() => {
    const names = new Map<string, string>();
    for (const repository of repositoryEntities ?? []) names.set(repository.id, repository.name);
    return names;
  }, [repositoryEntities]);
  return useMemo(
    () => buildRoots(repositories, session, repositoryNames),
    [repositories, session, repositoryNames],
  );
}

export function useTaskRepositoryRoots(
  taskId: string | null,
  sessionId: string | null,
): TaskRepositoryRoots {
  return useTaskRepositoryRootsFromIds(taskId, sessionId);
}

export function useActiveTaskRepositoryRoots(): TaskRepositoryRoots {
  const taskId = useAppStore((state) => state.tasks?.activeTaskId ?? null);
  const sessionId = useAppStore((state) => state.tasks?.activeSessionId ?? null);
  return useTaskRepositoryRootsFromIds(taskId, sessionId);
}

export function useTaskRepositoryRootsForSession(sessionId: string | null): TaskRepositoryRoots {
  const taskId = useAppStore((state) => {
    if (!sessionId) return null;
    return state.taskSessions?.items?.[sessionId]?.task_id ?? state.tasks?.activeTaskId ?? null;
  });
  return useTaskRepositoryRootsFromIds(taskId, sessionId);
}
