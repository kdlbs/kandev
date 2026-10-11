"use client";

import { useEffect, useState } from "react";
import { readJourneyWorkflowSteps } from "@/hooks/journey-metadata-resources";
import { useOptionalAppStore, useOptionalAppStoreApi } from "@/components/state-provider";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { t } from "@/lib/i18n";

export type WorkflowStepOption = { id: string; name: string };

// useWorkflowSteps fetches the steps for one workflow and exposes a loading
// flag so callers can disable the step Select while the fetch is in flight.
//
// The previous in-file copies in the watcher dialogs deferred the empty-state
// reset via Promise.resolve to dodge a setState-in-effect lint rule, which
// left a small window where the step dropdown rendered with the previous
// workflow's steps right after the user picked a new workflow. The version
// here resets synchronously using React's "store information from previous
// renders" pattern, so the new workflow's steps are never preceded by stale
// content.
export function useWorkflowSteps(workflowId: string): {
  steps: WorkflowStepOption[];
  loading: boolean;
} {
  const store = useOptionalAppStoreApi() ?? undefined;
  const scope = useOptionalAppStore(stateReadScopeIdentity, "");
  const scopeKey = JSON.stringify([workflowId, scope]);
  const [steps, setSteps] = useState<WorkflowStepOption[]>([]);
  // Initialize loading to match the effect's behaviour on first render: if
  // workflowId is truthy at mount we'll fetch immediately, so the dropdown
  // should show "Loading steps…" rather than "No steps in this workflow"
  // before the fetch lands. Only the setState-during-render guard below ever
  // toggles loading back on for subsequent workflowId changes.
  const [loading, setLoading] = useState(!!workflowId);
  const [prevWorkflowId, setPrevWorkflowId] = useState(scopeKey);

  // setState-during-render is the React-blessed way to derive state from a
  // changing input without a useEffect race. React drops the in-progress
  // render and re-renders with the new state immediately.
  if (prevWorkflowId !== scopeKey) {
    setPrevWorkflowId(scopeKey);
    setSteps([]);
    setLoading(!!workflowId);
  }

  useEffect(() => {
    if (!workflowId) return;
    let cancelled = false;
    const controller = new AbortController();
    readJourneyWorkflowSteps(store, workflowId, { signal: controller.signal })
      .then((res) => {
        if (cancelled) return;
        const sorted = [...res.steps].sort((a, b) => a.position - b.position);
        setSteps(sorted.map((s) => ({ id: s.id, name: s.name })));
      })
      .catch(() => {
        if (!cancelled) setSteps([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [workflowId, scopeKey, store]);

  return { steps, loading };
}

// stepPlaceholder picks the right empty-state text for the step Select based
// on whether a workflow has been chosen, whether its steps are still loading,
// and whether the chosen workflow has any steps at all.
export function stepPlaceholder(
  workflowId: string,
  stepsLoading: boolean,
  stepsCount: number,
): string {
  if (!workflowId) return t("workflows:selectAWorkflowFirst");
  if (stepsLoading) return t("workflows:loadingSteps");
  if (stepsCount === 0) return t("workflows:noStepsInThisWorkflow");
  return t("workflows:selectStep");
}
