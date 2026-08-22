"use client";

import { useTranslation } from "react-i18next";
import type { WorkflowStep } from "@/components/kanban-column";
import { areAllEmptyStepsAutoHidden } from "@/lib/kanban/auto-hide-empty-columns";

export function DesktopAutoHiddenEmptyState() {
  const { t } = useTranslation();
  return (
    <div
      className="mx-3 mb-3 rounded-lg border border-dashed border-border/70 px-4 py-6 text-center text-sm text-muted-foreground"
      data-testid="kanban-auto-hidden-empty-state"
    >
      {t("kanban:allEmptyStepsAutoHidden")}
    </div>
  );
}

export function getDesktopEmptyState(
  isMobile: boolean,
  displaySteps: WorkflowStep[],
  moveTargetSteps: WorkflowStep[],
) {
  if (isMobile || displaySteps.length > 0) return undefined;
  if (!areAllEmptyStepsAutoHidden(displaySteps, moveTargetSteps)) return null;
  return <DesktopAutoHiddenEmptyState />;
}
