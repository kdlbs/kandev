"use client";

import { memo, useCallback, useMemo } from "react";
import { IconCheck, IconFolder, IconGitBranch } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import type { Repository, TaskSession } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type RepoRow = {
  taskRepositoryId: string;
  repositoryId: string;
  name: string;
  baseBranch: string;
  checkoutBranch?: string;
  sessionCount: number;
  /** A representative session for this repo, if any (preferring primary). */
  switchToSessionId: string | null;
};

function pickSwitchToSession(
  sessions: TaskSession[],
  repositoryId: string,
  primarySessionId: string | null | undefined,
): string | null {
  const onRepo = sessions.filter((s) => s.repository_id === repositoryId);
  if (onRepo.length === 0) return null;
  if (primarySessionId && onRepo.some((s) => s.id === primarySessionId)) return primarySessionId;
  // Most recently started session on this repo as fallback.
  const sorted = [...onRepo].sort(
    (a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
  );
  return sorted[0].id;
}

function buildRepoRows(
  taskRepositories: NonNullable<KanbanState["tasks"][number]["repositories"]>,
  workspaceRepos: Repository[],
  taskSessions: TaskSession[],
  primarySessionId: string | null | undefined,
): RepoRow[] {
  return [...taskRepositories]
    .sort((a, b) => a.position - b.position)
    .map((tr) => {
      const repo = workspaceRepos.find((r) => r.id === tr.repository_id);
      const onRepoSessions = taskSessions.filter((s) => s.repository_id === tr.repository_id);
      return {
        taskRepositoryId: tr.id,
        repositoryId: tr.repository_id,
        name: repo?.name ?? repo?.local_path ?? "Repository",
        baseBranch: tr.base_branch,
        checkoutBranch: tr.checkout_branch,
        sessionCount: onRepoSessions.length,
        switchToSessionId: pickSwitchToSession(taskSessions, tr.repository_id, primarySessionId),
      };
    });
}

function useTaskRepoRows(taskId: string | null, workspaceId: string | null): RepoRow[] {
  const taskRepositories = useAppStore((s) => {
    if (!taskId) return undefined;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.repositories;
  });
  const workspaceRepos = useAppStore((s) =>
    workspaceId ? (s.repositories.itemsByWorkspaceId[workspaceId] ?? []) : [],
  );
  const taskSessions = useAppStore((s) =>
    taskId ? (s.taskSessionsByTask.itemsByTaskId[taskId] ?? []) : [],
  );
  const primarySessionId = useAppStore((s) => {
    if (!taskId) return null;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.primarySessionId ?? null;
  });
  return useMemo(
    () =>
      taskRepositories
        ? buildRepoRows(taskRepositories, workspaceRepos, taskSessions, primarySessionId)
        : [],
    [taskRepositories, workspaceRepos, taskSessions, primarySessionId],
  );
}

function RepoRowItem({
  row,
  isActive,
  onSelect,
}: {
  row: RepoRow;
  isActive: boolean;
  onSelect: (row: RepoRow) => void;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSelect(row)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(row);
        }
      }}
      data-testid={`mobile-repo-row-${row.repositoryId}`}
      className={`flex items-center gap-2 px-2 py-2 rounded-md cursor-pointer select-none ${
        isActive ? "bg-accent" : "hover:bg-accent/50"
      }`}
    >
      <IconFolder className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="flex flex-col min-w-0 flex-1">
        <span className="text-sm truncate">{row.name}</span>
        <div className="flex items-center gap-1 min-w-0">
          <IconGitBranch className="h-3 w-3 text-muted-foreground shrink-0" />
          <span className="text-[11px] text-muted-foreground truncate">
            {row.checkoutBranch || row.baseBranch}
          </span>
          {row.sessionCount > 0 && (
            <span className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-foreground/10 text-muted-foreground leading-none ml-1 shrink-0">
              {row.sessionCount} session{row.sessionCount === 1 ? "" : "s"}
            </span>
          )}
        </div>
      </div>
      {isActive && <IconCheck className="h-4 w-4 text-foreground shrink-0" />}
    </div>
  );
}

export const MobileReposSection = memo(function MobileReposSection({
  taskId,
  workspaceId,
  onClose,
}: {
  taskId: string | null;
  workspaceId: string | null;
  onClose: () => void;
}) {
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const activeRepoId = useAppStore((s) =>
    activeSessionId ? (s.taskSessions.items[activeSessionId]?.repository_id ?? null) : null,
  );
  const rows = useTaskRepoRows(taskId, workspaceId);
  const { toast } = useToast();

  const handleSelect = useCallback(
    (row: RepoRow) => {
      if (!taskId) return;
      if (row.switchToSessionId) {
        setActiveSession(taskId, row.switchToSessionId);
        onClose();
        return;
      }
      // We don't auto-launch from a repo tap — that would create a side effect
      // for what looks like navigation. Surface a hint so the tap doesn't
      // appear silently broken.
      toast({
        title: "No session on this repo yet",
        description: "Open the session picker to launch one.",
      });
    },
    [taskId, setActiveSession, onClose, toast],
  );

  if (!taskId) {
    return (
      <div className="text-xs text-muted-foreground px-2 py-6 text-center">No active task</div>
    );
  }
  if (rows.length === 0) {
    return (
      <div className="text-xs text-muted-foreground px-2 py-6 text-center">
        This task has no repositories.
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-0.5 px-1">
      {rows.map((row) => (
        <RepoRowItem
          key={row.taskRepositoryId}
          row={row}
          isActive={row.repositoryId === activeRepoId}
          onSelect={handleSelect}
        />
      ))}
    </div>
  );
});

/** Returns the count of repositories attached to the active task. */
export function useTaskRepoCount(taskId: string | null): number {
  return (
    useAppStore((s) => {
      if (!taskId) return 0;
      const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
      return task?.repositories?.length ?? 0;
    }) ?? 0
  );
}
