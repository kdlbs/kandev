import { useCallback, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getTaskPRsForCurrentWorkspace,
  useTaskPRTooltipHydration,
} from "./use-task-pr-tooltip-hydration";
import { useTaskPRUnlink } from "./use-task-pr-unlink";

export type TaskPRUnlinkChoice = {
  associationId: string;
  owner: string;
  repo: string;
  number: number;
  pending: boolean;
};

export function useTaskPRUnlinkMenu(taskId: string, compactPRNumber: number | null | undefined) {
  const taskPRs = useAppStore((state) => getTaskPRsForCurrentWorkspace(state, taskId));
  const hydration = useTaskPRTooltipHydration(taskId);
  const unlinkAction = useTaskPRUnlink(taskId);
  const [menuOpen, setMenuOpen] = useState(false);
  const hasCompactPR = typeof compactPRNumber === "number" && compactPRNumber > 0;

  const onOpenChange = useCallback(
    (open: boolean) => {
      setMenuOpen(open);
      if (open && hasCompactPR && taskPRs === null) void hydration.hydrate();
    },
    [hasCompactPR, hydration.hydrate, taskPRs],
  );

  const choices = useMemo<TaskPRUnlinkChoice[]>(
    () =>
      (taskPRs ?? []).map((pr) => ({
        associationId: pr.id,
        owner: pr.owner,
        repo: pr.repo,
        number: pr.pr_number,
        pending: unlinkAction.pendingIds.has(pr.id),
      })),
    [taskPRs, unlinkAction.pendingIds],
  );
  const isLoading =
    menuOpen && hasCompactPR && taskPRs === null && hydration.status !== "unavailable";

  return {
    choices,
    isLoading,
    onOpenChange,
    unlink: unlinkAction.unlink,
    canUnlink: unlinkAction.canUnlink,
  };
}
