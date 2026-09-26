"use client";

import { useState, useMemo } from "react";
import { IconChevronDown, IconLoader2 } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { invalidateCumulativeDiffCache } from "@/hooks/domains/session/use-cumulative-diff";
import { updateTaskRepositoryBaseBranch } from "@/lib/api/domains/kanban-api";
import { useToast } from "@/components/toast-provider";
import { repositoryId, type Repository } from "@/lib/types/http";
import { BranchPickerList } from "./branch-picker-list";
import { useTranslation } from "react-i18next";

type ResolvedRepo = {
  taskRepositoryId: string;
  storedBase: string;
  workspaceId: string;
  repositoryId: string;
};

/**
 * Resolves the WorkspaceTracker's `repositoryName` (= worktree dir basename)
 * to the task_repositories row + workspace Repository pair. For single-repo
 * tasks the empty `repositoryName` falls back to the only row.
 */
function useResolvedTaskRepo(taskId: string | null, repositoryName: string): ResolvedRepo | null {
  const task = useAppStore((s) => (taskId ? s.kanban.tasks.find((t) => t.id === taskId) : null));
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  return useMemo(() => {
    if (!task?.repositories?.length) return null;
    const allRepos = Object.values(reposByWorkspace).flat() as Repository[];
    if (repositoryName === "" && task.repositories.length === 1) {
      const link = task.repositories[0]!;
      const repo = allRepos.find((r) => r.id === repositoryId(link.repository_id));
      return repo
        ? {
            taskRepositoryId: link.id,
            storedBase: link.base_branch,
            workspaceId: repo.workspace_id,
            repositoryId: repo.id,
          }
        : null;
    }
    const repo = allRepos.find((r) => r.name === repositoryName);
    if (!repo) return null;
    const link = task.repositories.find((l) => repositoryId(l.repository_id) === repo.id);
    return link
      ? {
          taskRepositoryId: link.id,
          storedBase: link.base_branch,
          workspaceId: repo.workspace_id,
          repositoryId: repo.id,
        }
      : null;
  }, [task, reposByWorkspace, repositoryName]);
}

/**
 * Shared open/save/handleSelect/branch-load logic used by both picker
 * variants (inline span replacement + always-visible header button). Keeps
 * the trigger components tiny and avoids duplicated select + toast plumbing.
 */
function usePickerLogic(taskId: string | null, repositoryName: string, fallbackBaseBranch: string) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const resolved = useResolvedTaskRepo(taskId, repositoryName);
  const sessionId = useEnvironmentSessionId();
  const envKey = useAppStore((s) =>
    sessionId ? (s.environmentIdBySessionId[sessionId] ?? sessionId) : null,
  );
  const bumpSessionCommitsRefetch = useAppStore((s) => s.bumpSessionCommitsRefetch);

  const { branches, isLoading: isLoadingBranches } = useBranches(
    resolved
      ? { kind: "id", workspaceId: resolved.workspaceId, repositoryId: resolved.repositoryId }
      : null,
    open,
  );

  const currentBase = resolved?.storedBase || fallbackBaseBranch;

  const handleSelect = async (branch: string) => {
    // currentBase falls back to fallbackBaseBranch when storedBase is empty
    // (legacy task before any picker change). Compare against currentBase so
    // selecting the displayed value is treated as a no-op instead of saving
    // a spurious update.
    if (!resolved || !taskId || branch === currentBase) {
      setOpen(false);
      return;
    }
    setIsSaving(true);
    try {
      await updateTaskRepositoryBaseBranch(taskId, resolved.taskRepositoryId, branch);
      // Backend already cleared session.base_commit_sha and rewrote
      // session.base_branch; nudge the client-side caches so the commits
      // panel + cumulative diff refetch against the new base instead of
      // serving the stale snapshot until next page load.
      if (sessionId) bumpSessionCommitsRefetch(sessionId);
      if (envKey) invalidateCumulativeDiffCache(envKey);
    } catch (err) {
      toast({
        title: t("task:failedToChangeCompareBranch"),
        description: err instanceof Error ? err.message : t("task:unknownError"),
        variant: "error",
      });
    } finally {
      setIsSaving(false);
      setOpen(false);
    }
  };

  return {
    open,
    setOpen,
    isSaving,
    resolved,
    branches,
    isLoadingBranches,
    currentBase,
    handleSelect,
  };
}

/**
 * Compare-against picker rendered inline inside the branch hover card.
 * Replaces the static base branch label with a click-to-edit trigger.
 */
export function BaseBranchPicker({
  taskId,
  repositoryName,
  fallbackBaseBranch,
}: {
  taskId: string | null;
  repositoryName: string;
  fallbackBaseBranch: string;
}) {
  const logic = usePickerLogic(taskId, repositoryName, fallbackBaseBranch);

  if (!logic.resolved) {
    return <span className="text-foreground font-medium">{logic.currentBase}</span>;
  }

  return (
    <Popover open={logic.open} onOpenChange={logic.setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-haspopup="listbox"
          aria-expanded={logic.open}
          data-testid="base-branch-picker-trigger"
          className={cn(
            "inline-flex items-center gap-1 cursor-pointer rounded px-1 py-px text-foreground font-medium hover:bg-muted",
            logic.isSaving && "opacity-60 cursor-wait",
          )}
          disabled={logic.isSaving}
        >
          <span className="truncate max-w-[12rem]">{logic.currentBase}</span>
          {logic.isSaving ? (
            <IconLoader2 className="h-2.5 w-2.5 animate-spin" />
          ) : (
            <IconChevronDown className="h-2.5 w-2.5 opacity-50" />
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-64 p-1 max-h-72 overflow-auto"
        role="listbox"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <BranchPickerList
          branches={logic.branches}
          isLoadingBranches={logic.isLoadingBranches}
          currentBase={logic.currentBase}
          onSelect={logic.handleSelect}
        />
      </PopoverContent>
    </Popover>
  );
}
