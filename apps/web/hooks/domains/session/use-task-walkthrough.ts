"use client";

import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore, useOptionalAppStore } from "@/components/state-provider";
import { taskWalkthroughQueryOptions } from "@/lib/query/query-options/session";
import type { TaskWalkthrough } from "@/lib/types/http";

export function useTaskWalkthrough(taskId: string | null | undefined, enabled = true) {
  return useQuery(taskWalkthroughQueryOptions(taskId, enabled));
}

export function useActiveTaskWalkthrough() {
  const taskId = useOptionalAppStore((s) => s.tasks.activeTaskId, null);
  return {
    taskId,
    ...useTaskWalkthrough(taskId),
  };
}

export function useWalkthroughStepState(taskId: string | null | undefined, stepCount: number) {
  const activeStep = useOptionalAppStore(
    (s) => (taskId ? (s.walkthroughs.activeStepByTaskId[taskId] ?? 0) : 0),
    0,
  );
  const setActiveStepState = useAppStore((s) => s.setWalkthroughActiveStep);
  const setActiveStep = useCallback(
    (nextStep: number) => {
      if (!taskId) return;
      setActiveStepState(taskId, nextStep, stepCount);
    },
    [setActiveStepState, stepCount, taskId],
  );

  useEffect(() => {
    if (!taskId || stepCount === 0) return;
    if (activeStep < stepCount) return;
    setActiveStepState(taskId, stepCount - 1, stepCount);
  }, [activeStep, setActiveStepState, stepCount, taskId]);

  return { activeStep, setActiveStep };
}

export function useWalkthroughSeen(
  taskId: string | null | undefined,
  walkthrough: TaskWalkthrough | null | undefined,
) {
  const lastSeenUpdatedAt = useOptionalAppStore(
    (s) => (taskId ? s.walkthroughs.lastSeenUpdatedAtByTaskId[taskId] : undefined),
    undefined,
  );
  const hydrateLastSeen = useAppStore((s) => s.hydrateWalkthroughLastSeen);
  const markSeenState = useAppStore((s) => s.markWalkthroughSeen);

  useEffect(() => {
    if (taskId) hydrateLastSeen(taskId);
  }, [hydrateLastSeen, taskId]);

  const markSeen = useCallback(() => {
    if (!taskId) return;
    markSeenState(taskId, walkthrough?.updated_at ?? "");
  }, [markSeenState, taskId, walkthrough?.updated_at]);

  return {
    lastSeenUpdatedAt,
    hasUnseen: Boolean(walkthrough && walkthrough.updated_at !== lastSeenUpdatedAt),
    markSeen,
  };
}
