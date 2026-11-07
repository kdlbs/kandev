"use client";

import { useCallback, useEffect, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  getEffectiveTaskListingView,
  getStoredTaskListingView,
  resolveTaskListingView,
  setStoredTaskListingView,
  TASK_LISTING_VIEW_CHANGE_EVENT,
  type TaskListingView,
} from "@/lib/task-listing/view-preference";

function subscribe(callback: () => void) {
  window.addEventListener(TASK_LISTING_VIEW_CHANGE_EVENT, callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener(TASK_LISTING_VIEW_CHANGE_EVENT, callback);
    window.removeEventListener("storage", callback);
  };
}

/** Board view each listing view selects (`userSettings.kanbanViewMode`). */
const BOARD_VIEW_MODES: Partial<Record<TaskListingView, string>> = {
  pipeline: "graph2",
  graph: "graph",
};

export function useTaskListingView() {
  const { isMobile } = useResponsiveBreakpoint();
  const legacyKanbanViewMode = useAppStore((state) => state.userSettings.kanbanViewMode);
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const preferredView = useSyncExternalStore(
    subscribe,
    () => resolveTaskListingView(getStoredTaskListingView(), legacyKanbanViewMode),
    () => "kanban" as TaskListingView,
  );
  const effectiveView = getEffectiveTaskListingView(preferredView, isMobile);

  useEffect(() => {
    const kanbanViewMode = BOARD_VIEW_MODES[effectiveView] ?? null;
    if (userSettings.kanbanViewMode === kanbanViewMode) return;
    setUserSettings({ ...userSettings, kanbanViewMode });
  }, [effectiveView, setUserSettings, userSettings]);

  const setView = useCallback((view: TaskListingView) => {
    setStoredTaskListingView(view);
  }, []);

  return { preferredView, effectiveView, setView };
}
