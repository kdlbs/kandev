"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";

/**
 * Watches the active task's plan and opens the Plan panel quietly (without
 * stealing focus from the current session) whenever the agent has written a
 * new version the user hasn't seen.
 *
 * Reactive-effect placement is important: the WS event and `activeTaskId`
 * being set in the store are a race at page-load time, so doing this in the
 * WS handler (which sees only the event moment) loses events. Running as an
 * effect keyed on `[activeTaskId, plan.updated_at, lastSeen]` catches both
 * orderings.
 */
export function usePlanPanelAutoOpen() {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const plan = useAppStore((s) => (activeTaskId ? s.taskPlans.byTaskId[activeTaskId] : null));
  const lastSeen = useAppStore((s) =>
    activeTaskId ? s.taskPlans.lastSeenUpdatedAtByTaskId[activeTaskId] : undefined,
  );
  const api = useDockviewStore((s) => s.api);
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);

  useEffect(() => {
    if (!api || isRestoringLayout) return;
    if (!plan || plan.created_by !== "agent") return;
    if (lastSeen === plan.updated_at) return;
    if (api.getPanel("plan")) return;

    addPlanPanel({ quiet: true, inCenter: true });
  }, [api, isRestoringLayout, plan, lastSeen, addPlanPanel]);
}
