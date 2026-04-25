"use client";

import { useEffect, useLayoutEffect } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { useAppStore } from "@/components/state-provider";

/**
 * Custom tab component for the Plan panel.
 * Shows a small indicator dot when the agent has written/updated the plan but
 * the user hasn't focused the Plan panel yet. Focusing the tab clears it.
 */
export function PlanTab(props: IDockviewPanelHeaderProps) {
  const { api } = props;

  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const plan = useAppStore((s) => (activeTaskId ? s.taskPlans.byTaskId[activeTaskId] : null));
  const lastSeen = useAppStore((s) =>
    activeTaskId ? s.taskPlans.lastSeenUpdatedAtByTaskId[activeTaskId] : undefined,
  );
  const markTaskPlanSeen = useAppStore((s) => s.markTaskPlanSeen);

  // Clear the indicator when the tab becomes active.
  useEffect(() => {
    const disposable = api.onDidActiveChange((event) => {
      if (event.isActive && activeTaskId) markTaskPlanSeen(activeTaskId);
    });
    return () => disposable.dispose();
  }, [api, activeTaskId, markTaskPlanSeen]);

  // If the tab is already active when the plan changes (user is viewing it),
  // treat updates as immediately seen. Use useLayoutEffect so the seen-mark
  // commits before paint — otherwise the dot flashes for one frame between
  // the WS update render and the seen-mark render.
  const planUpdatedAt = plan?.updated_at;
  useLayoutEffect(() => {
    if (api.isActive && activeTaskId) markTaskPlanSeen(activeTaskId);
  }, [api, activeTaskId, markTaskPlanSeen, planUpdatedAt]);

  const hasUnseen = plan?.created_by === "agent" && lastSeen !== plan.updated_at;

  return (
    <div data-testid="plan-tab" className="relative">
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
