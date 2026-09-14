"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { useTaskMoveOptions, type TaskMoveStep } from "../task-move-context-menu";
import { WorkflowMoveOptionsForm, type WorkflowMoveOptionsSubmit } from "../workflow-move-options";

export type MobileMoveOptionsRequest = {
  taskId: string;
  workflowId: string;
  targetStep: TaskMoveStep;
};

export function useMobileTaskMoveOptions({
  open,
  stepsByWorkflowId,
}: {
  open: boolean;
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
}) {
  const [request, setRequest] = useState<MobileMoveOptionsRequest | null>(null);
  const moveOptions = useTaskMoveOptions({
    taskId: request?.taskId ?? "",
    workflowId: request?.workflowId,
    steps: request ? [request.targetStep] : undefined,
  });
  const handleRequest = useCallback(
    (taskId: string, workflowId: string, targetStepId: string) => {
      const targetStep = stepsByWorkflowId[workflowId]?.find((step) => step.id === targetStepId);
      if (!targetStep) return;
      const targetWithWorkflow = { ...targetStep, workflow_id: workflowId, task_id: taskId };
      setRequest({ taskId, workflowId, targetStep: targetWithWorkflow });
      moveOptions.openMoveOptionsStep(targetWithWorkflow);
    },
    [moveOptions.openMoveOptionsStep, stepsByWorkflowId],
  );
  const handleClose = useCallback(() => {
    moveOptions.closeMoveOptions();
    setRequest(null);
  }, [moveOptions.closeMoveOptions]);
  const handleSubmit = useCallback(
    async (entryOptions: Parameters<WorkflowMoveOptionsSubmit>[0]) => {
      const moved = await moveOptions.submitMoveOptions(entryOptions);
      if (moved) setRequest(null);
      return moved;
    },
    [moveOptions.submitMoveOptions],
  );

  useEffect(() => {
    if (open || !request) return;
    moveOptions.closeMoveOptions();
    setRequest(null);
  }, [moveOptions.closeMoveOptions, open, request]);

  return {
    request,
    moveOptionsStep: moveOptions.moveOptionsStep,
    isMoving: moveOptions.isMoving,
    handleRequest,
    handleClose,
    handleSubmit,
  };
}

export function MobileTaskMoveOptionsSurface({
  presentation,
  step,
  isMoving,
  onBack,
  onSubmit,
}: {
  presentation: "sheet" | "drawer";
  step: TaskMoveStep;
  isMoving: boolean;
  onBack: () => void;
  onSubmit: WorkflowMoveOptionsSubmit;
}) {
  const { t } = useTranslation();
  const header = (
    <div className="flex items-start gap-2">
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="min-h-11 min-w-11 shrink-0 cursor-pointer"
        aria-label={t("common:back")}
        onClick={onBack}
      >
        <IconArrowLeft className="h-4 w-4" />
      </Button>
      <div className="min-w-0 pt-2">
        {presentation === "drawer" ? (
          <DrawerTitle className="text-base">
            {t("task:workflowMoveOptionsTitle", { step: step.title })}
          </DrawerTitle>
        ) : (
          <SheetTitle className="text-base">
            {t("task:workflowMoveOptionsTitle", { step: step.title })}
          </SheetTitle>
        )}
        <p className="pt-1 text-sm text-muted-foreground">
          {t("task:workflowMoveOptionsDescription")}
        </p>
      </div>
    </div>
  );
  return (
    <>
      {presentation === "drawer" ? (
        <DrawerHeader className="shrink-0 border-b border-border p-4 text-left">
          {header}
        </DrawerHeader>
      ) : (
        <SheetHeader className="shrink-0 border-b border-border p-4 text-left">
          {header}
        </SheetHeader>
      )}
      <div
        className="min-h-0 flex-1 overflow-y-auto p-4"
        data-vaul-no-drag={presentation === "drawer" ? true : undefined}
        data-testid="workflow-move-options"
      >
        <WorkflowMoveOptionsForm
          isMoving={isMoving}
          isTouchSurface={presentation === "drawer"}
          onSubmit={onSubmit}
          onCancel={onBack}
        />
      </div>
    </>
  );
}
