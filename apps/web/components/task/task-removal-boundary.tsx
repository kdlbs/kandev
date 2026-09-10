"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { GridSpinner } from "@/components/grid-spinner";
import { useAppStore } from "@/components/state-provider";
import { taskRemovalCoversTask } from "@/lib/state/task-removal";

type TaskRemovalBoundaryProps = {
  taskId: string | null | undefined;
  children: ReactNode;
};

function TaskRemovalStatus() {
  const { t } = useTranslation();
  const headingRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    headingRef.current?.focus();
  }, []);

  return (
    <div
      className="flex h-full min-h-0 w-full items-center justify-center bg-background px-4"
      data-testid="task-removal-status"
      role="status"
      aria-live="polite"
    >
      <div
        ref={headingRef}
        className="flex min-h-24 min-w-0 flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground outline-none"
        tabIndex={-1}
      >
        <span aria-hidden="true">
          <GridSpinner className="text-primary" />
        </span>
        <span>{t("common:taskRemovalInProgress")}</span>
      </div>
    </div>
  );
}

export function TaskRemovalBoundary({ taskId, children }: TaskRemovalBoundaryProps) {
  const isPending = useAppStore((state) => {
    if (taskId && taskRemovalCoversTask(state.taskRemoval, taskId)) return true;
    const activeTaskId = state.tasks.activeTaskId;
    return activeTaskId ? taskRemovalCoversTask(state.taskRemoval, activeTaskId) : false;
  });

  return isPending ? <TaskRemovalStatus /> : children;
}
