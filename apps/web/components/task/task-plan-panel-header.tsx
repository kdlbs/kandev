"use client";

import { useCallback, useState } from "react";
import { PanelHeaderBarSplit } from "./panel-primitives";
import { TaskPlanRevisions } from "./task-plan-revisions";
import { getPlanToolbarImplementState } from "./task-plan-implement";
import { ImplementPlanButton } from "./chat/implement-plan-button";
import { useImplementPlanRunner } from "@/hooks/domains/kanban/use-plan-actions";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";

type PlanPanelHeaderProps = {
  taskId: string;
  plan: TaskPlan | null;
  draftContent: string;
  hasUnsavedChanges: boolean;
  activeSessionId: string | null;
  revisions: TaskPlanRevision[];
  isLoadingRevisions: boolean;
  isSaving: boolean;
  isAgentBusy: boolean;
  savePlan: (content: string, title?: string) => Promise<TaskPlan | null>;
  onOpenRevisions: () => void;
  onRevert: (id: string) => Promise<TaskPlanRevision | null>;
  loadRevisionContent: (revisionId: string) => Promise<string>;
  previewRevisionId: string | null;
  setPreviewRevision: (revisionId: string | null) => void;
  comparePair: [string | null, string | null];
  toggleCompareSelection: (revisionId: string) => void;
  clearComparePair: () => void;
};

// Header bar lives only to host plan actions. The plan title is already shown
// by the dockview/mobile tab above the panel, so we don't repeat it here.
export function PlanPanelHeader({
  taskId,
  plan,
  draftContent,
  hasUnsavedChanges,
  activeSessionId,
  revisions,
  isLoadingRevisions,
  isSaving,
  isAgentBusy,
  savePlan,
  onOpenRevisions,
  onRevert,
  loadRevisionContent,
  previewRevisionId,
  setPreviewRevision,
  comparePair,
  toggleCompareSelection,
  clearComparePair,
}: PlanPanelHeaderProps) {
  const [isImplementing, setIsImplementing] = useState(false);
  const implementPlan = useImplementPlanRunner({
    resolvedSessionId: activeSessionId,
    taskId,
    clearPlanModeAfterSend: false,
  });
  const implementState = getPlanToolbarImplementState({ draftContent, plan });
  const implementDisabled =
    implementState.disabled || isSaving || isImplementing || isAgentBusy || !activeSessionId;
  const handleImplement = useCallback(
    async (fresh: boolean) => {
      if (implementDisabled) return;
      setIsImplementing(true);
      try {
        let savedPlan = plan;
        if (hasUnsavedChanges || !plan) {
          savedPlan = await savePlan(draftContent, plan?.title);
          if (!savedPlan) return;
        }
        if (!savedPlan) return;
        const savedState = getPlanToolbarImplementState({
          draftContent: savedPlan.content,
          plan: savedPlan,
        });
        if (!savedState.visible || savedState.disabled) {
          return;
        }
        await implementPlan(fresh);
      } finally {
        setIsImplementing(false);
      }
    },
    [draftContent, hasUnsavedChanges, implementDisabled, implementPlan, plan, savePlan],
  );

  return (
    <PanelHeaderBarSplit
      left={null}
      right={
        <>
          {implementState.visible && (
            <ImplementPlanButton
              onClick={handleImplement}
              disabled={implementDisabled}
              disabledReason={implementState.disabledReason}
              framed
              testIds={{
                root: "plan-toolbar-implement-control",
                button: "plan-toolbar-implement-button",
                menuTrigger: "plan-toolbar-implement-menu-trigger",
                freshItem: "plan-toolbar-implement-fresh-menu-item",
              }}
            />
          )}
          <TaskPlanRevisions
            taskId={taskId}
            revisions={revisions}
            isLoading={isLoadingRevisions}
            isSaving={isSaving}
            onOpen={onOpenRevisions}
            onRevert={onRevert}
            loadRevisionContent={loadRevisionContent}
            previewRevisionId={previewRevisionId}
            setPreviewRevision={setPreviewRevision}
            comparePair={comparePair}
            toggleCompareSelection={toggleCompareSelection}
            clearComparePair={clearComparePair}
          />
        </>
      }
    />
  );
}
