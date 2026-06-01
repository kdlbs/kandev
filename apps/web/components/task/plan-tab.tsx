"use client";

import { useCallback, useEffect, useLayoutEffect } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { taskPlanQueryOptions, type TaskPlanData } from "@/lib/query/query-options/session";
import { qk } from "@/lib/query/keys";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";

/**
 * Custom tab component for the Plan panel.
 * Shows a small indicator dot when the agent has written/updated the plan but
 * the user hasn't focused the Plan panel yet. Focusing the tab clears it.
 */
export function PlanTab(props: IDockviewPanelHeaderProps) {
  const { api } = props;
  const onDoubleClick = useTabMaximizeOnDoubleClick(api);

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
  const queryClient = useQueryClient();

  // Mark the active task's plan as seen, reading the freshest updated_at from
  // the TQ cache so async callbacks don't capture a stale closure value.
  const markActivePlanSeen = useCallback(() => {
    if (!activeTaskId) return;
    const current = queryClient.getQueryData<TaskPlanData>(qk.taskSession.plans(activeTaskId));
    markTaskPlanSeen(activeTaskId, current?.plan?.updated_at);
  }, [activeTaskId, queryClient, markTaskPlanSeen]);

  // Clear the indicator when the tab becomes active.
  useEffect(() => {
    const disposable = api.onDidActiveChange((event) => {
      if (event.isActive) markActivePlanSeen();
    });
    return () => disposable.dispose();
  }, [api, markActivePlanSeen]);

  // If the tab is already active when the plan changes (user is viewing it),
  // treat updates as immediately seen. Use useLayoutEffect so the seen-mark
  // commits before paint — otherwise the dot flashes for one frame between
  // the WS update render and the seen-mark render.
  const planUpdatedAt = plan?.updated_at;
  useLayoutEffect(() => {
    if (api.isActive) markActivePlanSeen();
  }, [api, markActivePlanSeen, planUpdatedAt]);

  const hasUnseen = plan?.created_by === "agent" && lastSeen !== plan.updated_at;

  return (
    <div
      data-testid="plan-tab"
      className="relative cursor-pointer select-none"
      onDoubleClick={onDoubleClick}
    >
      <DockviewDefaultTab {...props} />
      {hasUnseen && (
        <span
          data-testid="plan-tab-indicator"
          className="absolute top-0.5 left-0 size-2 rounded-full bg-primary pointer-events-none"
        />
      )}
    </div>
  );
}
