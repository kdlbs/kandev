"use client";

import { useEffect, useRef, useState } from "react";
import { TaskArchiveConfirmation } from "./task-archive-confirmation";
import type { TaskSwitcherItem } from "./task-switcher-types";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type TaskSwitcherArchiveConfirmationOptions = {
  task: TaskSwitcherItem;
  onArchiveTask?: (taskId: string, opts?: { cascade?: boolean }) => void;
  isArchiving?: boolean;
  closeMenu: () => void;
};

export function useTaskSwitcherArchiveConfirmation({
  task,
  onArchiveTask,
  isArchiving,
  closeMenu,
}: TaskSwitcherArchiveConfirmationOptions) {
  const { isMobile } = useResponsiveBreakpoint();
  const [archiveOpen, setArchiveOpen] = useState(false);
  const pendingPhoneArchive = useRef(false);
  const archiveAnchorRef = useRef<HTMLDivElement>(null);
  const openTimerRef = useRef<number | null>(null);
  useEffect(
    () => () => {
      if (openTimerRef.current !== null) window.clearTimeout(openTimerRef.current);
    },
    [],
  );
  const requestArchive = onArchiveTask
    ? () => {
        if (isMobile) {
          pendingPhoneArchive.current = true;
          return;
        }
        closeMenu();
        openTimerRef.current = window.setTimeout(() => {
          openTimerRef.current = null;
          setArchiveOpen(true);
        }, 0);
      }
    : undefined;
  const archiveConfirmation =
    onArchiveTask && archiveOpen ? (
      <TaskArchiveConfirmation
        open
        anchorRef={archiveAnchorRef}
        focusReturnRef={archiveAnchorRef}
        taskId={task.id}
        taskTitle={task.title}
        executorType={task.remoteExecutorType}
        isArchiving={isArchiving}
        isInFlight={task.foregroundActivity !== null && task.foregroundActivity !== undefined}
        onOpenChange={setArchiveOpen}
        onConfirm={({ cascade }) => onArchiveTask(task.id, { cascade })}
      />
    ) : undefined;

  const handleMenuCloseAutoFocus = (event: Event) => {
    if (!pendingPhoneArchive.current) return;
    event.preventDefault();
    pendingPhoneArchive.current = false;
    setArchiveOpen(true);
  };
  return {
    archiveOpen,
    archiveAnchorRef,
    requestArchive,
    archiveConfirmation,
    handleMenuCloseAutoFocus,
  };
}
