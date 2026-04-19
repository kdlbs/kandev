import React, { useCallback, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { setChatDraftContent } from "@/lib/local-storage";
import { moveTask } from "@/lib/api/domains/kanban-api";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";

const PLAN_CONTEXT_PATH = "plan:context";

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

  // Track agent switching: isMoving stays true from "proceed" click until the
  // new session is adopted (activeSessionId changes from the original).
  const [moveFromSessionId, setMoveFromSessionId] = useState<string | null>(null);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const isMoving = moveFromSessionId != null && activeSessionId === moveFromSessionId;

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
    const capturedSessionId = activeSessionId;
    setMoveFromSessionId(capturedSessionId);
    try {
      await moveTask(taskId, {
        workflow_id: workflowId,
        workflow_step_id: nextStep.id,
        position: 0,
      });
      // Safety: if the next step reuses the same session (no agent-profile
      // override), activeSessionId never changes and isMoving would be stuck.
      // Clear after 10 s if no session handoff occurred.
      setTimeout(() => {
        setMoveFromSessionId((prev) => (prev === capturedSessionId ? null : prev));
      }, 10_000);
    } catch (err) {
      console.error("Failed to proceed to next step:", err);
      toast({ description: "Failed to proceed to next step", variant: "error" });
      setMoveFromSessionId(null);
    }
  }, [taskId, workflowId, nextStep, activeSessionId, toast]);

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

/** Directly disable plan mode state + layout, bypassing the MCP availability guard. */
function useDirectDisablePlanMode(resolvedSessionId: string | null) {
  const setPlanMode = useAppStore((s) => s.setPlanMode);
  const setActiveDocument = useAppStore((s) => s.setActiveDocument);
  const closeDocument = useLayoutStore((s) => s.closeDocument);
  const removeContextFile = useContextFilesStore((s) => s.removeFile);
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);

  return useCallback(() => {
    if (!resolvedSessionId) return;
    applyBuiltInPreset("default");
    closeDocument(resolvedSessionId);
    setActiveDocument(resolvedSessionId, null);
    setPlanMode(resolvedSessionId, false);
    removeContextFile(resolvedSessionId, PLAN_CONTEXT_PATH);
  }, [
    resolvedSessionId,
    applyBuiltInPreset,
    closeDocument,
    setActiveDocument,
    setPlanMode,
    removeContextFile,
  ]);
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
  const {
    proceedStepName,
    nextStepIsWorkStep,
    proceed: rawProceed,
    isMoving,
  } = useNextWorkflowStep(opts.taskId);

  const disablePlanMode = useDirectDisablePlanMode(opts.resolvedSessionId);
  const { planModeEnabled } = opts;
  // Disable plan mode before proceeding so the layout switches back to default
  // and the next step's auto-start prompt is visible in chat.
  const proceed = useCallback(() => {
    if (planModeEnabled) {
      disablePlanMode();
    }
    rawProceed();
  }, [planModeEnabled, disablePlanMode, rawProceed]);

  const implementPlanHandler =
    opts.planModeEnabled && !nextStepIsWorkStep ? handleImplementPlan : undefined;
  return { implementPlanHandler, proceedStepName, proceed, isMoving };
}
