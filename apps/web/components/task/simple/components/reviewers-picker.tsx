"use client";

import { useMemo } from "react";
import { addTaskReviewer, removeTaskReviewer } from "@/lib/api/domains/office-extended-api";
import type { Task } from "@/app/office/tasks/[id]/types";
import { AgentsMultiPicker, buildDecisionLookup } from "./agents-multi-picker";

type ReviewersPickerProps = {
  task: Task;
};

export function ReviewersPicker({ task }: ReviewersPickerProps) {
  const decisionsByAgent = useMemo(
    () => buildDecisionLookup(task.decisions, "reviewer"),
    [task.decisions],
  );
  return (
    <AgentsMultiPicker
      task={task}
      selectedIds={task.reviewers}
      fieldKey="reviewers"
      addLabel="+ Add reviewer"
      testId="reviewers-picker-trigger"
      apiAdd={addTaskReviewer}
      apiRemove={removeTaskReviewer}
      decisionsByAgent={decisionsByAgent}
    />
  );
}
