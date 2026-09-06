"use client";

import { useContext, useMemo, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { useTask } from "@/hooks/use-task";
import { useSessionWorktrees } from "@/hooks/domains/session/use-session-worktrees";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  buildMarkdownFileRootAliases,
  type MarkdownAliasRepository,
  type MarkdownAliasWorktree,
} from "@/lib/markdown/file-link-target";
import {
  MarkdownFileLinkContext,
  type MarkdownFileLinkContextValue,
} from "@/components/shared/markdown-components";

type TaskRepositoryLink = {
  repository_id?: string | null;
};

type TaskLinkSource = {
  workspaceId?: string | null;
  repositories?: readonly TaskRepositoryLink[] | null;
  repositoryId?: string | null;
};

type LegacySessionWorktreeSource = {
  repository_id?: string | null;
  worktree_path?: string | null;
};

type TaskMarkdownFileLinkProviderProps = {
  taskId?: string | null;
  sessionId?: string | null;
  worktreePath?: string | null;
  onOpenFile?: (path: string) => void;
  children: ReactNode;
};

function collectTaskRepositoryIds(sources: readonly (TaskLinkSource | null | undefined)[]) {
  const ids = new Set<string>();
  for (const source of sources) {
    if (source?.repositoryId) ids.add(source.repositoryId);
    for (const repository of source?.repositories ?? []) {
      if (repository.repository_id) ids.add(repository.repository_id);
    }
  }
  return [...ids];
}

function toAliasRepositories(
  repositories: readonly { id: string; local_path?: string | null }[],
): MarkdownAliasRepository[] {
  return repositories.map((repository) => ({
    id: repository.id,
    localPath: repository.local_path,
  }));
}

function toAliasWorktrees(
  worktrees: readonly { repositoryId?: string; path?: string }[],
  legacySession?: LegacySessionWorktreeSource | null,
): MarkdownAliasWorktree[] {
  const aliases = worktrees.map((worktree) => ({
    repositoryId: worktree.repositoryId,
    path: worktree.path,
  }));
  if (aliases.length > 0 || !legacySession?.repository_id || !legacySession.worktree_path) {
    return aliases;
  }
  return [
    {
      repositoryId: legacySession.repository_id,
      path: legacySession.worktree_path,
    },
  ];
}

/**
 * Supplies one task/session-scoped Markdown file-link context to all message
 * renderers in an interactive transcript.
 */
export function TaskMarkdownFileLinkProvider({
  taskId,
  sessionId,
  worktreePath,
  onOpenFile,
  children,
}: TaskMarkdownFileLinkProviderProps) {
  const task = useTask(taskId ?? null) as TaskLinkSource | null;
  const officeTask = useAppStore((state) =>
    taskId ? (state.office.tasks.items.find((item) => item.id === taskId) ?? null) : null,
  ) as TaskLinkSource | null;
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const sessionWorktrees = useSessionWorktrees(sessionId ?? null);
  const inheritedContext = useContext(MarkdownFileLinkContext);

  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceId = task?.workspaceId ?? officeTask?.workspaceId ?? activeWorkspaceId;
  const taskRepositoryIds = useMemo(
    () => collectTaskRepositoryIds([task, officeTask]),
    [officeTask, task],
  );
  const { repositories: workspaceRepositories } = useRepositories(
    workspaceId,
    Boolean(workspaceId),
    true,
  );
  const effectiveWorkspacePath = worktreePath ?? getSessionWorkspacePath(session);
  const fileRootAliases = useMemo(
    () =>
      buildMarkdownFileRootAliases({
        workspaceRoot: effectiveWorkspacePath,
        taskRepositoryIds,
        repositories: toAliasRepositories(workspaceRepositories),
        sessionWorktrees: toAliasWorktrees(sessionWorktrees, session),
      }),
    [effectiveWorkspacePath, session, sessionWorktrees, taskRepositoryIds, workspaceRepositories],
  );
  const contextValue = useMemo<MarkdownFileLinkContextValue>(
    () => ({
      worktreePath: effectiveWorkspacePath ?? inheritedContext.worktreePath,
      onOpenFile: onOpenFile ?? inheritedContext.onOpenFile,
      fileRootAliases,
    }),
    [
      effectiveWorkspacePath,
      fileRootAliases,
      inheritedContext.onOpenFile,
      inheritedContext.worktreePath,
      onOpenFile,
    ],
  );

  return (
    <MarkdownFileLinkContext.Provider value={contextValue}>
      {children}
    </MarkdownFileLinkContext.Provider>
  );
}
