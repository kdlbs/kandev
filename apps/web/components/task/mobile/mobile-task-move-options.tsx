"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { SheetHeader, SheetTitle } from "@kandev/ui/sheet";
import { useAppStore } from "@/components/state-provider";
import { usePresentationToken } from "@/hooks/domains/kanban/use-workflow-step-move";
import { useWorkflowMove } from "@/hooks/domains/kanban/use-workflow-move";
import type { WorkflowMoveResponse } from "@/lib/api/domains/kanban-api";
import { useTaskMoveOptions, type TaskMoveStep } from "../task-move-context-menu";
import { WorkflowMoveOptionsForm, type WorkflowMoveOptionsSubmit } from "../workflow-move-options";
import { useToast } from "@/components/toast-provider";
import { getTaskMoveErrorMessage } from "../task-move-error-message";

export type MobileMoveOptionsRequest = {
  taskId: string;
  workflowId: string;
  targetStep: TaskMoveStep;
};

type MobileFocusRequest = {
  requestId: number;
  taskId: string;
  presentationToken: number;
};

function useMobileWorkflowSessionFocus(activeTaskId: string | null) {
  const presentationToken = usePresentationToken(activeTaskId);
  const navigationRevision = useAppStore((state) => state.taskRemoval.navigationRevision);
  const beginWorkflowSessionFocus = useAppStore((state) => state.beginWorkflowSessionFocus);
  const bindWorkflowSessionFocus = useAppStore((state) => state.bindWorkflowSessionFocus);
  const reconcileWorkflowSessionFocus = useAppStore((state) => state.reconcileWorkflowSessionFocus);
  const cancelWorkflowSessionFocus = useAppStore((state) => state.cancelWorkflowSessionFocus);
  const focusRequestRef = useRef<MobileFocusRequest | null>(null);

  const beginFocus = useCallback(
    (taskId: string, workflowId: string, destinationStepId: string) => {
      if (taskId !== activeTaskId) return null;
      const requestId = beginWorkflowSessionFocus({
        taskId,
        workflowId,
        destinationStepId,
        presentationToken,
        navigationRevision,
      });
      if (requestId === null) return null;
      focusRequestRef.current = { requestId, taskId, presentationToken };
      return requestId;
    },
    [activeTaskId, beginWorkflowSessionFocus, navigationRevision, presentationToken],
  );
  const cancelFocus = useCallback(
    (requestId: number) => {
      const focusRequest = focusRequestRef.current;
      if (!focusRequest || focusRequest.requestId !== requestId) return;
      cancelWorkflowSessionFocus({
        requestId: focusRequest.requestId,
        presentationToken: focusRequest.presentationToken,
      });
      focusRequestRef.current = null;
    },
    [cancelWorkflowSessionFocus],
  );
  const clearFocus = useCallback((requestId: number) => {
    if (focusRequestRef.current?.requestId === requestId) focusRequestRef.current = null;
  }, []);
  const cancelCurrentFocus = useCallback(() => {
    const focusRequest = focusRequestRef.current;
    if (!focusRequest) return;
    cancelWorkflowSessionFocus({
      requestId: focusRequest.requestId,
      presentationToken: focusRequest.presentationToken,
    });
    focusRequestRef.current = null;
  }, [cancelWorkflowSessionFocus]);
  const handleMoveCommitted = useCallback(
    (response: WorkflowMoveResponse, expectedRequestId?: number) => {
      const focusRequest = focusRequestRef.current;
      if (!focusRequest) return;
      if (expectedRequestId !== undefined && focusRequest.requestId !== expectedRequestId) return;
      if (!response.workflow_entry_identity) {
        cancelFocus(focusRequest.requestId);
        return;
      }
      bindWorkflowSessionFocus({
        requestId: focusRequest.requestId,
        presentationToken: focusRequest.presentationToken,
        entryIdentity: response.workflow_entry_identity,
      });
      reconcileWorkflowSessionFocus(focusRequest.taskId, {
        metadata: response.task?.metadata,
        updatedAt: response.task?.updated_at,
        workflowStepId: response.task?.workflow_step_id,
        entryIdentity: response.workflow_entry_identity,
      });
    },
    [bindWorkflowSessionFocus, cancelFocus, reconcileWorkflowSessionFocus],
  );

  useEffect(
    () => () => {
      const focusRequest = focusRequestRef.current;
      if (!focusRequest) return;
      cancelWorkflowSessionFocus({
        requestId: focusRequest.requestId,
        presentationToken: focusRequest.presentationToken,
      });
    },
    [cancelWorkflowSessionFocus],
  );

  return {
    beginFocus,
    cancelFocus,
    cancelCurrentFocus,
    clearFocus,
    handleMoveCommitted,
    navigationRevision,
    presentationToken,
  };
}

function useMobileDirectTaskMove({
  stepsByWorkflowId,
  focus,
}: {
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  focus: ReturnType<typeof useMobileWorkflowSessionFocus>;
}) {
  const { move, isMoving } = useWorkflowMove();
  const { toast } = useToast();
  const { t } = useTranslation("task");
  const handleMove = useCallback(
    (taskId: string, workflowId: string, targetStepId: string) => {
      const targetStep = stepsByWorkflowId[workflowId]?.find((step) => step.id === targetStepId);
      if (!targetStep) return;
      const focusRequestId = focus.beginFocus(taskId, workflowId, targetStepId);
      void move(taskId, {
        workflow_id: workflowId,
        workflow_step_id: targetStepId,
        position: 0,
      }).then((result) => {
        if (result.disposition === "failed") {
          if (focusRequestId !== null) focus.cancelFocus(focusRequestId);
          const fallback = t("taskMoveErrorGeneric");
          toast({
            title: t("failedToMoveTask"),
            description: getTaskMoveErrorMessage(result.error, fallback, t),
            variant: "error",
          });
          return;
        }
        if (focusRequestId !== null) {
          focus.handleMoveCommitted(result.response, focusRequestId);
          focus.clearFocus(focusRequestId);
        }
        toast({
          title: t("movedTasksToStep", { count: 1 }),
          description: t("movedTasksStepDescription", { count: 1 }),
          variant: "success",
        });
      });
    },
    [focus, move, stepsByWorkflowId, t, toast],
  );

  return { handleMove, isMoving };
}

export function useMobileTaskMoveOptions({
  open,
  activeTaskId,
  stepsByWorkflowId,
}: {
  open: boolean;
  activeTaskId: string | null;
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
}) {
  const [request, setRequest] = useState<MobileMoveOptionsRequest | null>(null);
  const focus = useMobileWorkflowSessionFocus(activeTaskId);
  const directMove = useMobileDirectTaskMove({ stepsByWorkflowId, focus });
  const moveOptions = useTaskMoveOptions({
    taskId: request?.taskId ?? "",
    workflowId: request?.workflowId,
    steps: request ? [request.targetStep] : undefined,
    onMoveCommitted: focus.handleMoveCommitted,
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
      const focusRequestId = request?.workflowId
        ? focus.beginFocus(request.taskId, request.workflowId, request.targetStep.id)
        : null;
      const moved = await moveOptions.submitMoveOptions(entryOptions);
      if (!moved && focusRequestId !== null) focus.cancelFocus(focusRequestId);
      if (moved && focusRequestId !== null) focus.clearFocus(focusRequestId);
      if (moved) setRequest(null);
      return moved;
    },
    [focus, moveOptions.submitMoveOptions, request],
  );

  useEffect(() => {
    if (open || !request) return;
    moveOptions.closeMoveOptions();
    setRequest(null);
    focus.cancelCurrentFocus();
  }, [focus.cancelCurrentFocus, moveOptions.closeMoveOptions, open, request]);

  return {
    request,
    moveOptionsStep: moveOptions.moveOptionsStep,
    isMoving: moveOptions.isMoving || directMove.isMoving,
    handleRequest,
    handleMove: directMove.handleMove,
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
