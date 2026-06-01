"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { taskPlanQueryOptions } from "@/lib/query/query-options/session";

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
 *
 * The plan itself now comes from TanStack Query. Mounting the query here (with
 * `enabled` once the active task is known) doubles as the eager fetch that used
 * to live in a manual effect: the Plan panel mounts `useTaskPlan` only after
 * the panel exists, so without this the plan written by the agent before the
 * browser's WS connected (fast auto-start path) would never populate the cache
 * and the auto-open below would never fire.
 */
export function usePlanPanelAutoOpen() {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const planQuery = useQuery({
    ...taskPlanQueryOptions(activeTaskId ?? ""),
    enabled: !!activeTaskId,
  });
  const plan = planQuery.data?.plan ?? null;
  const lastSeen = useAppStore((s) =>
    activeTaskId ? s.taskPlans.lastSeenUpdatedAtByTaskId[activeTaskId] : undefined,
  );
  const markTaskPlanSeen = useAppStore((s) => s.markTaskPlanSeen);
  const api = useDockviewStore((s) => s.api);
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);

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
      // cache directly and re-arms the indicator.
      if (lastSeen === undefined) markTaskPlanSeen(plan.task_id, plan.updated_at);
      return;
    }

    addPlanPanel({ quiet: true, inCenter: true });
  }, [api, isRestoringLayout, plan, lastSeen, addPlanPanel, markTaskPlanSeen]);
}
