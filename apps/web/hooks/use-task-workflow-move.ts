"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import { bulkMoveSelectedTasks, getBulkMoveApiError, type BulkMoveTaskRefusal } from "@/lib/api";
import { useToast } from "@/components/toast-provider";
import { getTaskMoveErrorMessage } from "@/components/task/task-move-error-message";

export function useTaskWorkflowMove() {
  const { toast } = useToast();
  const { t, i18n } = useTranslation("task");

  return useCallback(
    async (
      taskIds: string[],
      targetWorkflowId: string,
      targetStepId: string,
      destination: "step" | "workflow" = "workflow",
    ) => {
      const ids = [...new Set(taskIds.filter(Boolean))];
      if (ids.length === 0) return;
      try {
        const result = await bulkMoveSelectedTasks({
          task_ids: ids,
          target_workflow_id: targetWorkflowId,
          target_step_id: targetStepId,
        });
        toast({
          title: t(destination === "step" ? "task:movedTasksToStep" : "task:movedTasksToWorkflow", {
            count: result.moved_count,
          }),
          description: t(
            destination === "step"
              ? "task:movedTasksStepDescription"
              : "task:movedTasksWorkflowDescription",
            { count: result.moved_count },
          ),
          variant: "success",
        });
        return result;
      } catch (error) {
        const bulkMoveError = getBulkMoveApiError(error);
        if (bulkMoveError && bulkMoveError.result.refusals.length > 0) {
          const { result } = bulkMoveError;
          const firstRefusal = result.refusals[0];
          if (result.code === "bulk_move_partial") {
            toast({
              title: t("task:bulkMovePartialTitle", {
                moved: result.moved_task_ids.length,
                total: ids.length,
              }),
              description: t("task:bulkMovePartialDescription", {
                task: firstRefusal.task_title,
                reason: bulkMoveRefusalReason(firstRefusal, t, i18n.language),
              }),
              variant: "error",
            });
          } else {
            const lines = result.refusals.slice(0, 3).map((refusal) =>
              t("task:bulkMoveRefusalLine", {
                title: refusal.task_title,
                reason: bulkMoveRefusalReason(refusal, t, i18n.language),
              }),
            );
            if (result.refusals.length > lines.length) {
              lines.push(
                t("task:bulkMoveAndMore", { count: result.refusals.length - lines.length }),
              );
            }
            const items = new Intl.ListFormat(i18n.language, {
              style: "short",
              type: "conjunction",
            }).format(lines);
            toast({
              title: t("task:bulkMoveNoTasksMoved"),
              description: t("task:bulkMoveRefusedDescription", { items }),
              variant: "error",
            });
          }
          throw error;
        }
        const fallback = t("task:taskMoveErrorGeneric");
        toast({
          title: t("task:failedToMoveTask"),
          description: getTaskMoveErrorMessage(error, fallback, t),
          variant: "error",
        });
        throw error;
      }
    },
    [i18n.language, t, toast],
  );
}

function bulkMoveRefusalReason(
  refusal: BulkMoveTaskRefusal,
  translate: (key: string, options?: Record<string, unknown>) => string,
  language: string,
) {
  if (refusal.code === "task_approvals_pending") {
    const names = refusal.pending_approvers?.map((approver) => approver.name).filter(Boolean) ?? [];
    if (names.length > 0) {
      const approvers = new Intl.ListFormat(language, {
        style: "short",
        type: "conjunction",
      }).format(names);
      return translate("task:bulkMoveApprovalPending", { approvers });
    }
  }
  return getTaskMoveErrorMessage(
    new ApiError(refusal.message, 409, { code: refusal.code }),
    translate("task:taskMoveErrorGeneric"),
    (key) => translate(key),
  );
}
