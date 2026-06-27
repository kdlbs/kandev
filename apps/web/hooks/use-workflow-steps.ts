"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { workflowStepsQueryOptions } from "@/lib/query/query-options";

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
  const query = useQuery(workflowStepsQueryOptions(workflowId));
  const steps = useMemo(
    () => (query.data ?? []).map((step) => ({ id: step.id, name: step.name })),
    [query.data],
  );

  return { steps, loading: Boolean(workflowId) && query.isFetching && steps.length === 0 };
}

// stepPlaceholder picks the right empty-state text for the step Select based
// on whether a workflow has been chosen, whether its steps are still loading,
// and whether the chosen workflow has any steps at all.
export function stepPlaceholder(
  workflowId: string,
  stepsLoading: boolean,
  stepsCount: number,
): string {
  if (!workflowId) return "Select a workflow first";
  if (stepsLoading) return "Loading steps…";
  if (stepsCount === 0) return "No steps in this workflow";
  return "Select step";
}
