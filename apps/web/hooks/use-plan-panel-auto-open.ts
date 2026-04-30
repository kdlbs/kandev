"use client";

import { useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
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
  const storeApi = useAppStoreApi();
  const api = useDockviewStore((s) => s.api);
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);

  // Track tasks we've already attempted to fetch so a transient failure
  // doesn't put us in an infinite retry loop. Cleared on WS disconnect so a
  // reconnect can retry any failed or not-yet-attempted tasks.
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
      .then((fetched) => {
        // Race guard: if a WS event populated the store while our HTTP
        // request was in flight, don't overwrite a real plan with a
        // stale response — neither a `null` (server didn't have it yet
        // at fetch time) nor an older non-null version (HTTP saw an
        // earlier write than the WS event we already applied).
        const live = storeApi.getState().taskPlans.byTaskId[taskId];
        if (live) {
          if (fetched === null) return;
          if (Date.parse(fetched.updated_at) < Date.parse(live.updated_at)) return;
        }
        setTaskPlan(taskId, fetched);
      })
      .catch(() => {
        /* swallow — the disconnect/reconnect effect clears `attemptedRef`
         * so a transient failure retries automatically after recovery. */
      })
      .finally(() => setTaskPlanLoading(taskId, false));
  }, [activeTaskId, connectionStatus, isLoaded, setTaskPlan, setTaskPlanLoading, storeApi]);

  // Clear the attempt set on WS disconnect so that when the WS reconnects
  // the fetch effect can retry any tasks that previously failed or were pending.
  useEffect(() => {
    if (connectionStatus === "connected") return;
    attemptedRef.current.clear();
  }, [connectionStatus]);

  useEffect(() => {
    if (!api || isRestoringLayout) return;
    if (!plan || plan.created_by !== "agent") return;
    if (lastSeen === plan.updated_at) return;
    if (api.getPanel("plan")) {
      // Page-reload case: panel restored from saved layout and there is no
      // recorded `lastSeen` (not persisted across sessions). We can't tell
      // whether the plan was acknowledged before reload or not, so we
      // optimistically mark it seen to avoid a stale indicator flash on
      // every reload. When a live update arrives after the reload,
      // `lastSeen` is defined so we don't suppress it — PlanTab reads the
      // store directly and re-arms the indicator.
      if (lastSeen === undefined) markTaskPlanSeen(plan.task_id);
      return;
    }

    addPlanPanel({ quiet: true, inCenter: true });
  }, [api, isRestoringLayout, plan, lastSeen, addPlanPanel, markTaskPlanSeen]);
}
