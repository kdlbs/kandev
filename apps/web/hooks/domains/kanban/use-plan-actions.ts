import React, { useCallback, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { setChatDraftContent } from "@/lib/local-storage";
import { moveTask } from "@/lib/api/domains/kanban-api";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";

const AUTO_TRANSITION_ACTIONS = ["move_to_next", "move_to_previous", "move_to_step"];

function useNextWorkflowStep(taskId: string | null) {
  const { toast } = useToast();
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const steps = useAppStore((s) => s.kanban.steps);
  const taskStepId = useAppStore((s) => {
    if (!taskId) return null;
    const task = s.kanban.tasks.find((t) => t.id === taskId);
    return task?.workflowStepId ?? null;
  });

  const [isMoving, setIsMoving] = useState(false);

  const sortedSteps = useMemo(() => [...steps].sort((a, b) => a.position - b.position), [steps]);

  const { currentStep, nextStep } = useMemo(() => {
    const currentIndex = sortedSteps.findIndex((s) => s.id === taskStepId);
    const current = currentIndex >= 0 ? sortedSteps[currentIndex] : null;
    const next =
      currentIndex >= 0 && currentIndex < sortedSteps.length - 1
        ? sortedSteps[currentIndex + 1]
        : null;
    return { currentStep: current, nextStep: next };
  }, [sortedSteps, taskStepId]);

  const currentStepAutoTransitions = useMemo(
    () =>
      currentStep?.events?.on_turn_complete?.some((a) =>
        AUTO_TRANSITION_ACTIONS.includes(a.type),
      ) ?? false,
    [currentStep],
  );

  const nextStepIsWorkStep = useMemo(() => {
    if (!nextStep) return false;
    const hasAutoStart =
      nextStep.events?.on_enter?.some((a) => a.type === "auto_start_agent") ?? false;
    const hasPlanMode =
      nextStep.events?.on_enter?.some((a) => a.type === "enable_plan_mode") ?? false;
    return hasAutoStart && !hasPlanMode;
  }, [nextStep]);

  const proceed = useCallback(async () => {
    if (!taskId || !workflowId || !nextStep) return;
    setIsMoving(true);
    try {
      await moveTask(taskId, {
        workflow_id: workflowId,
        workflow_step_id: nextStep.id,
        position: 0,
      });
      // Keep isMoving=true until the WS event updates taskStepId and
      // proceedStepName becomes null, hiding the button entirely.
    } catch (err) {
      console.error("Failed to proceed to next step:", err);
      toast({ description: "Failed to proceed to next step", variant: "error" });
      setIsMoving(false);
    }
  }, [taskId, workflowId, nextStep, toast]);

  const proceedStepName = nextStep && !currentStepAutoTransitions ? nextStep.title : null;

  return { proceedStepName, nextStepIsWorkStep, proceed, isMoving };
}

function useImplementPlan(
  resolvedSessionId: string | null,
  taskId: string | null,
  handlePlanModeChange: (enabled: boolean) => void,
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>,
) {
  return useCallback(() => {
    if (!resolvedSessionId || !taskId) return;

    const client = getWebSocketClient();
    if (!client) return;

    const userText = chatInputRef.current?.getValue() ?? "";
    chatInputRef.current?.clear();
    if (resolvedSessionId) {
      setChatDraftContent(resolvedSessionId, null);
    }

    const visibleText = userText.trim() || "Implement the plan";
    const content = `${visibleText}\n\n<kandev-system>
IMPLEMENT PLAN: The user has approved the plan and wants you to implement it now.
Read the current plan using the get_task_plan_kandev MCP tool.
Implement all changes described in the plan step by step.
After completing the implementation, provide a summary of what was done.
</kandev-system>`;

    client
      .request(
        "message.add",
        {
          task_id: taskId,
          session_id: resolvedSessionId,
          content,
          plan_mode: false,
        },
        10000,
      )
      .catch((err: unknown) => console.error("Failed to send implement plan message:", err));

    handlePlanModeChange(false);
  }, [resolvedSessionId, taskId, handlePlanModeChange, chatInputRef]);
}

export function usePlanActions(opts: {
  resolvedSessionId: string | null;
  taskId: string | null;
  planModeEnabled: boolean;
  handlePlanModeChange: (enabled: boolean) => void;
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>;
}) {
  const handleImplementPlan = useImplementPlan(
    opts.resolvedSessionId,
    opts.taskId,
    opts.handlePlanModeChange,
    opts.chatInputRef,
  );
  const { proceedStepName, nextStepIsWorkStep, proceed, isMoving } = useNextWorkflowStep(
    opts.taskId,
  );
  const implementPlanHandler =
    opts.planModeEnabled && !nextStepIsWorkStep ? handleImplementPlan : undefined;
  return { implementPlanHandler, proceedStepName, proceed, isMoving };
}
