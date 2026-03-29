import { useCallback, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { moveTask } from "@/lib/api/domains/kanban-api";

const AUTO_TRANSITION_ACTIONS = ["move_to_next", "move_to_previous", "move_to_step"];

export function useNextWorkflowStep(taskId: string | null) {
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
    } catch (err) {
      throw new Error("Failed to proceed to next step", { cause: err });
    } finally {
      setIsMoving(false);
    }
  }, [taskId, workflowId, nextStep]);

  // Show proceed when there's a next step and current step doesn't auto-transition
  const proceedStepName = nextStep && !currentStepAutoTransitions ? nextStep.title : null;

  return {
    proceedStepName,
    nextStepIsWorkStep,
    proceed,
    isMoving,
  };
}
