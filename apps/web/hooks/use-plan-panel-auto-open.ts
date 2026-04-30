"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getTaskPlan } from "@/lib/api/domains/plan-api";

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
  const isLoaded = useAppStore((s) =>
    activeTaskId ? (s.taskPlans.loadedByTaskId[activeTaskId] ?? false) : false,
  );
  const lastSeen = useAppStore((s) =>
    activeTaskId ? s.taskPlans.lastSeenUpdatedAtByTaskId[activeTaskId] : undefined,
  );
  const setTaskPlan = useAppStore((s) => s.setTaskPlan);
  const setTaskPlanLoading = useAppStore((s) => s.setTaskPlanLoading);
  const markTaskPlanSeen = useAppStore((s) => s.markTaskPlanSeen);
  const connectionStatus = useAppStore((s) => s.connection.status);
  const api = useDockviewStore((s) => s.api);
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);

  // Track tasks we've already attempted to fetch so a transient failure
  // doesn't put us in an infinite retry loop (the in-flight `loading` flag
  // would otherwise re-trigger the effect via dep updates). Cleared per
  // task only on successful navigation away.
  const attemptedRef = useRef<Set<string>>(new Set());

  // Eagerly fetch the plan on task load. The Plan panel mounts `useTaskPlan`
  // only after the panel exists, so without this fetch a plan written by the
  // agent before the browser's WS connected (fast auto-start path) would never
  // populate the store and the auto-open below would never fire.
  useEffect(() => {
    if (!activeTaskId || connectionStatus !== "connected") return;
    if (isLoaded) return;
    if (attemptedRef.current.has(activeTaskId)) return;
    const taskId = activeTaskId;
    attemptedRef.current.add(taskId);
    setTaskPlanLoading(taskId, true);
    getTaskPlan(taskId)
      .then((fetched) => setTaskPlan(taskId, fetched))
      .catch(() => {
        /* swallow — `useTaskPlan` retries on panel mount and on the next
         * WS reconnect via its own connectionStatus-keyed effect. */
      })
      .finally(() => setTaskPlanLoading(taskId, false));
  }, [activeTaskId, connectionStatus, isLoaded, setTaskPlan, setTaskPlanLoading]);

  // Drop the attempt mark when the WS reconnects so a transient failure can
  // be retried after recovery.
  useEffect(() => {
    if (connectionStatus === "connected") return;
    attemptedRef.current.clear();
  }, [connectionStatus]);

  useEffect(() => {
    if (!api || isRestoringLayout) return;
    if (!plan || plan.created_by !== "agent") return;
    if (lastSeen === plan.updated_at) return;
    if (api.getPanel("plan")) {
      // Page-reload case: panel restored from saved layout AND we have no
      // recorded `lastSeen` (cold hydrate). The plan was already acknowledged
      // before the reload — mark seen so a stale indicator doesn't flash.
      // Live updates keep `lastSeen` populated and re-arm normally.
      // `activeTaskId` is non-null here because `plan` was derived from it.
      if (lastSeen === undefined) markTaskPlanSeen(plan.task_id);
      return;
    }

    addPlanPanel({ quiet: true, inCenter: true });
  }, [api, isRestoringLayout, plan, lastSeen, addPlanPanel, markTaskPlanSeen]);
}
