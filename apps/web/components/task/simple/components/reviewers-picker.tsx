"use client";

import { useMemo } from "react";
import { addTaskReviewer, removeTaskReviewer } from "@/lib/api/domains/office-extended-api";
import type { Task } from "@/app/office/tasks/[id]/types";
import { AgentsMultiPicker, buildDecisionLookup } from "./agents-multi-picker";
import { useTranslation } from "react-i18next";

type ReviewersPickerProps = {
  task: Task;
};

export function ReviewersPicker({ task }: ReviewersPickerProps) {
  const { t } = useTranslation();
  const decisionsByAgent = useMemo(
    () => buildDecisionLookup(task.decisions, "reviewer"),
    [task.decisions],
  );
  return (
    <AgentsMultiPicker
      task={task}
      selectedIds={task.reviewers}
      fieldKey="reviewers"
      addLabel={t("task:addReviewer")}
      testId="reviewers-picker-trigger"
      apiAdd={addTaskReviewer}
      apiRemove={removeTaskReviewer}
      decisionsByAgent={decisionsByAgent}
    />
  );
}
