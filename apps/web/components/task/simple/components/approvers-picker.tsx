"use client";

import { useMemo } from "react";
import { addTaskApprover, removeTaskApprover } from "@/lib/api/domains/office-extended-api";
import type { Task } from "@/app/office/tasks/[id]/types";
import { AgentsMultiPicker, buildDecisionLookup } from "./agents-multi-picker";
import { useTranslation } from "react-i18next";

type ApproversPickerProps = {
  task: Task;
};

export function ApproversPicker({ task }: ApproversPickerProps) {
  const { t } = useTranslation();
  const decisionsByAgent = useMemo(
    () => buildDecisionLookup(task.decisions, "approver"),
    [task.decisions],
  );
  return (
    <AgentsMultiPicker
      task={task}
      selectedIds={task.approvers}
      fieldKey="approvers"
      addLabel={t("task:addApprover")}
      testId="approvers-picker-trigger"
      apiAdd={addTaskApprover}
      apiRemove={removeTaskApprover}
      decisionsByAgent={decisionsByAgent}
    />
  );
}
