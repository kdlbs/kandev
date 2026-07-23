"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { prTaskKey } from "@/components/github/pr-utils";
import type { TaskPR } from "@/lib/types/github";
import { getPrimaryTaskPR } from "./use-task-pr";

const EMPTY_PRS: TaskPR[] = [];

export function findReviewPRByKey(prs: TaskPR[], selectedKey: string): TaskPR | null {
  return prs.find((pr) => prTaskKey(pr) === selectedKey) ?? null;
}

export function resolveReviewPRSelection(prs: TaskPR[], selectedKey?: string): TaskPR | null {
  if (!selectedKey) return getPrimaryTaskPR(prs);
  return findReviewPRByKey(prs, selectedKey) ?? getPrimaryTaskPR(prs);
}

export function useReviewPRSelection(taskId: string | null) {
  const prs = useAppStore((state) =>
    taskId ? (state.taskPRs.byTaskId[taskId] ?? EMPTY_PRS) : EMPTY_PRS,
  );
  const storedKey = useAppStore((state) =>
    taskId ? state.reviewPRSelection.selectedKeyByTaskId[taskId] : undefined,
  );
  const setReviewPRSelection = useAppStore((state) => state.setReviewPRSelection);
  const selectedPR = resolveReviewPRSelection(prs, storedKey);
  const selectedKey = selectedPR ? prTaskKey(selectedPR) : null;
  const selectPR = useCallback(
    (pr: TaskPR) => {
      if (taskId) setReviewPRSelection(taskId, prTaskKey(pr));
    },
    [taskId, setReviewPRSelection],
  );

  return { prs, selectedPR, selectedKey, selectPR };
}
