"use client";

import { IconLoader2 } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { selectLiveSessionForTask } from "@/lib/state/slices/session/selectors";
import { useActiveSessionRef } from "./active-session-ref-context";
import { useTranslation } from "react-i18next";

type TopbarWorkingIndicatorProps = {
  taskId: string;
};

/**
 * Renders `<spinner /> Working` next to the task title in the page topbar
 * while the task has any live session (RUNNING / WAITING_FOR_INPUT).
 * Hidden otherwise — no layout reservation.
 *
 * Click scrolls the active session's timeline entry into view.
 */
export function TopbarWorkingIndicator({ taskId }: TopbarWorkingIndicatorProps) {
  const { t } = useTranslation();
  const liveSession = useAppStore((s) => selectLiveSessionForTask(s, taskId));
  const { getActiveNode } = useActiveSessionRef();

  if (!liveSession) return null;

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
      <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
      <span data-testid="topbar-working-active">{t("task:working3")}</span>
    </button>
  );
}
