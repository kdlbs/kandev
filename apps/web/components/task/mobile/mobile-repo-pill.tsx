"use client";

import { memo, useEffect, useMemo, useState } from "react";
import { IconFolder } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { MobilePillButton } from "./mobile-pill-button";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import { MobileReposSection, useTaskRepoCount } from "./mobile-repos-section";

const COMPACT_VIEWPORT_PX = 360;

/** Returns true once the viewport is narrower than the compact threshold. */
function useIsCompactViewport(): boolean {
  const [isCompact, setIsCompact] = useState(false);
  useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${COMPACT_VIEWPORT_PX - 1}px)`);
    const update = () => setIsCompact(mql.matches);
    update();
    mql.addEventListener("change", update);
    return () => mql.removeEventListener("change", update);
  }, []);
  return isCompact;
}

function useTaskActiveRepoName(taskId: string | null, workspaceId: string | null): string | null {
  const workspaceRepos = useAppStore((s) =>
    workspaceId ? (s.repositories.itemsByWorkspaceId[workspaceId] ?? []) : [],
  );
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const activeRepoId = useAppStore((s) =>
    activeSessionId ? (s.taskSessions.items[activeSessionId]?.repository_id ?? null) : null,
  );
  const taskRepos = useAppStore((s) => {
    if (!taskId) return undefined;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.repositories;
  });
  return useMemo(() => {
    if (!activeRepoId) {
      // Fallback to the position-primary task repo when no session is active
      // yet. Match the picker's ordering (mobile-repos-section sorts by
      // position) so the pill label and the first sheet row agree.
      const sorted = taskRepos ? [...taskRepos].sort((a, b) => a.position - b.position) : [];
      const fallback = sorted[0]?.repository_id;
      if (!fallback) return null;
      const repo = workspaceRepos.find((r) => r.id === fallback);
      return repo?.name ?? repo?.local_path ?? null;
    }
    const repo = workspaceRepos.find((r) => r.id === activeRepoId);
    return repo?.name ?? repo?.local_path ?? null;
  }, [activeRepoId, taskRepos, workspaceRepos]);
}

export const MobileRepoPill = memo(function MobileRepoPill({
  taskId,
  workspaceId,
}: {
  taskId: string | null;
  workspaceId: string | null;
}) {
  const repoCount = useTaskRepoCount(taskId);
  const activeName = useTaskActiveRepoName(taskId, workspaceId);
  const isCompact = useIsCompactViewport();
  const [open, setOpen] = useState(false);

  if (repoCount <= 1) return null;

  const label = activeName ?? "Repo";
  return (
    <>
      <MobilePillButton
        icon={<IconFolder className="h-3.5 w-3.5 shrink-0" />}
        label={label}
        compact={isCompact}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="mobile-repo-pill"
        ariaLabel={`Active repository: ${label}. Tap to switch.`}
      />
      <MobilePickerSheet open={open} onOpenChange={setOpen} title="Repositories">
        <MobileReposSection
          taskId={taskId}
          workspaceId={workspaceId}
          onClose={() => setOpen(false)}
        />
      </MobilePickerSheet>
    </>
  );
});
