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

type PendingRouteIdentity = {
  taskId: string | null;
  activeTaskIdBeforeRouteChange: string | null;
};

function useDisplayedTaskId(routeTaskId: string | null | undefined) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const routeTaskIdValue = routeTaskId ?? null;
  // A committed route owns the boundary until live task selection catches up;
  // unchanged routes follow sidebar or mobile in-place selection instead.
  const routeTaskIdRef = useRef(routeTaskIdValue);
  const pendingRouteIdentityRef = useRef<PendingRouteIdentity | null>(
    routeTaskIdValue !== null && routeTaskIdValue !== activeTaskId
      ? {
          taskId: routeTaskIdValue,
          activeTaskIdBeforeRouteChange: activeTaskId,
        }
      : null,
  );

  if (routeTaskIdRef.current !== routeTaskIdValue) {
    pendingRouteIdentityRef.current = {
      taskId: routeTaskIdValue,
      activeTaskIdBeforeRouteChange: activeTaskId,
    };
    routeTaskIdRef.current = routeTaskIdValue;
  }

  const pendingRouteIdentity = pendingRouteIdentityRef.current;
  if (
    pendingRouteIdentity &&
    (activeTaskId === pendingRouteIdentity.taskId ||
      (activeTaskId !== null &&
        activeTaskId !== pendingRouteIdentity.activeTaskIdBeforeRouteChange))
  ) {
    pendingRouteIdentityRef.current = null;
  }

  if (pendingRouteIdentityRef.current) return pendingRouteIdentityRef.current.taskId;
  return activeTaskId ?? routeTaskIdValue;
}

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
  const displayedTaskId = useDisplayedTaskId(taskId);
  const isPending = useAppStore((state) =>
    displayedTaskId ? taskRemovalCoversTask(state.taskRemoval, displayedTaskId) : false,
  );

  return isPending ? <TaskRemovalStatus /> : children;
}
