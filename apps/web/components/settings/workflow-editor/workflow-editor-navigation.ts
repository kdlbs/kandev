import { useCallback, useEffect } from "react";

import type { WorkflowStep } from "@/lib/types/http";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { markCurrentNavigationRoute } from "@/lib/routing/navigation-guard";
import type { WorkflowEditorIssue } from "@/lib/workflows/workflow-editor-view-model";
import type { WorkflowLifecycleTrigger } from "@/lib/workflows/workflow-action-catalog";
import {
  readWorkflowEditorSelection,
  workflowEditorSelectionPath,
  type WorkflowEditorRouteSelection,
  type WorkflowEditorTab,
} from "./workflow-editor-paths";

type WorkflowEditorNavigationMode = "push" | "replace";

export type WorkflowEditorActionFocus = {
  trigger: WorkflowLifecycleTrigger;
  index: number;
};

type WorkflowEditorNavigationDraft = {
  steps: WorkflowStep[];
  selectedStepId: string | null;
  selectedStep: WorkflowStep | null;
  onSelectStep: (stepId: string) => void;
};

export function useWorkflowEditorNavigation(draft: WorkflowEditorNavigationDraft) {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const routeSelection = readWorkflowEditorSelection(searchParams);
  const routeStep = routeSelection.stepId
    ? draft.steps.find((step) => step.id === routeSelection.stepId)
    : undefined;
  const selectedStep = routeStep ?? draft.selectedStep;
  const navigateSelection = useCallback(
    (selection: WorkflowEditorRouteSelection, mode: WorkflowEditorNavigationMode = "push") => {
      const href = workflowEditorSelectionPath(pathname, searchParams, selection);
      const query = searchParams.toString();
      const current = query ? `${pathname}?${query}` : pathname;
      if (href === current) return;
      const navigate = mode === "replace" ? router.replace : router.push;
      navigate(href, { scroll: false, shallow: true });
    },
    [pathname, router, searchParams],
  );

  useEffect(() => {
    markCurrentNavigationRoute(pathname);
  }, [pathname]);

  useEffect(() => {
    if (!routeStep || draft.selectedStepId === routeStep.id) return;
    draft.onSelectStep(routeStep.id);
  }, [draft.onSelectStep, draft.selectedStepId, routeStep]);

  const handleSelectStep = useCallback(
    (stepId: string) => {
      draft.onSelectStep(stepId);
      navigateSelection({
        ...routeSelection,
        stepId,
        trigger: null,
        actionIndex: null,
      });
    },
    [draft, navigateSelection, routeSelection],
  );
  const handleIssue = useCallback(
    (issue: WorkflowEditorIssue) => {
      if (issue.target === "workflow") {
        document.getElementById("workflow-editor-name")?.focus();
        return;
      }
      draft.onSelectStep(issue.stepId);
      navigateSelection({
        ...routeSelection,
        stepId: issue.stepId,
        tab: issue.trigger ? "automation" : routeSelection.tab,
        trigger: issue.trigger && issue.actionIndex !== undefined ? issue.trigger : null,
        actionIndex: issue.trigger && issue.actionIndex !== undefined ? issue.actionIndex : null,
      });
    },
    [draft, navigateSelection, routeSelection],
  );
  const handleTabChange = useCallback(
    (tab: WorkflowEditorTab) =>
      navigateSelection(
        {
          ...routeSelection,
          tab,
          trigger: null,
          actionIndex: null,
        },
        "replace",
      ),
    [navigateSelection, routeSelection],
  );
  const handleFocusAction = useCallback(
    (selection: WorkflowEditorActionFocus | null, mode: WorkflowEditorNavigationMode = "push") =>
      navigateSelection(
        {
          ...routeSelection,
          stepId: selectedStep?.id ?? routeSelection.stepId,
          tab: selection ? "automation" : routeSelection.tab,
          trigger: selection?.trigger ?? null,
          actionIndex: selection?.index ?? null,
        },
        mode,
      ),
    [navigateSelection, routeSelection, selectedStep],
  );

  return {
    activeTab: routeSelection.tab,
    focusedAction: focusedActionFor(selectedStep, routeSelection),
    hasStepSelection: routeSelection.stepId !== null,
    selectedStep,
    handleIssue,
    handleSelectStep,
    handleTabChange,
    handleFocusAction,
    onBackToJourney: () => router.back(),
    onBackToStep: () => router.back(),
  };
}

function focusedActionFor(
  step: WorkflowStep | null,
  selection: WorkflowEditorRouteSelection,
): WorkflowEditorActionFocus | null {
  if (!step || selection.trigger === null || selection.actionIndex === null) return null;
  if (selection.actionIndex < 0) return null;
  const actions = step.events?.[selection.trigger] ?? [];
  return actions[selection.actionIndex]
    ? { trigger: selection.trigger, index: selection.actionIndex }
    : null;
}
