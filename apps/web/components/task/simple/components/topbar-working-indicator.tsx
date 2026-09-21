"use client";

import { IconLoader2 } from "@tabler/icons-react";
import { CompositorSpin } from "@kandev/ui/compositor-spin";
import { useAppStore } from "@/components/state-provider";
import { selectLiveSessionForTask } from "@/lib/state/slices/session/selectors";
import { useActiveSessionRef } from "./active-session-ref-context";
import { useTranslation } from "react-i18next";
import type { TaskComment } from "../types";

type TopbarWorkingIndicatorProps = {
  taskId: string;
  comments?: TaskComment[];
};

/**
 * Renders `<spinner /> Working` next to the task title in the page topbar
 * while the task has any live session (RUNNING / WAITING_FOR_INPUT).
 * Hidden otherwise — no layout reservation.
 *
 * Click scrolls the active session's timeline entry into view.
 */
export function TopbarWorkingIndicator({ taskId, comments = [] }: TopbarWorkingIndicatorProps) {
  const { t } = useTranslation();
  const liveSession = useAppStore((s) => selectLiveSessionForTask(s, taskId));
  const { getActiveNode } = useActiveSessionRef();
  const pendingComment = comments.some(
    (comment) =>
      comment.authorType === "user" &&
      (comment.runStatus === "queued" || comment.runStatus === "claimed"),
  );

  if (!liveSession && !pendingComment) return null;
  const isWorking =
    Boolean(liveSession) || comments.some((comment) => comment.runStatus === "claimed");

  const handleClick = () => {
    const node = getActiveNode();
    if (!node) return;
    node.scrollIntoView({ block: "end", behavior: "smooth" });
  };

  return (
    <button
      type="button"
      onClick={handleClick}
      className="inline-flex items-center gap-1 text-xs text-primary cursor-pointer hover:opacity-80 transition-opacity"
      aria-label={t("task:scrollToActiveSession")}
      data-testid="topbar-working-indicator"
    >
      {isWorking ? (
        <CompositorSpin className="h-3.5 w-3.5">
          <IconLoader2 className="size-full" />
        </CompositorSpin>
      ) : (
        <span
          className="inline-block h-1.5 w-1.5 rounded-full bg-muted-foreground/60"
          aria-hidden
        />
      )}
      <span data-testid="topbar-working-active">
        {isWorking ? t("task:working3") : t("task:queued")}
      </span>
    </button>
  );
}
