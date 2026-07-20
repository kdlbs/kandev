"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import type {
  WorkflowStep,
  OnEnterAction,
  OnTurnStartAction,
  OnTurnCompleteAction,
  OnExitAction,
} from "@/lib/types/http";
import type { GenericAction } from "@/lib/types/workflow-actions";
import { cn } from "@/lib/utils";
import {
  HelpTip,
  getTransitionType,
  getOnTurnStartTransitionType,
  getChildrenCompletedTransitionType,
  hasDisablePlanMode,
} from "./workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "./workflow-dirty-state";

// --- useStepActions hook ---

type UseStepActionsArgs = {
  step: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
};

export function useStepActions({ step, onUpdate }: UseStepActionsArgs) {
  const toggleOnEnterAction = (type: string) => {
    const currentEvents = step.events ?? {};
    const onEnter = currentEvents.on_enter ?? [];
    const exists = onEnter.some((a) => a.type === type);
    const newOnEnter = exists
      ? onEnter.filter((a) => a.type !== type)
      : [...onEnter, { type } as OnEnterAction];
    onUpdate({ events: { ...currentEvents, on_enter: newOnEnter } });
  };

  const setTransition = (type: string) => {
    const currentEvents = step.events ?? {};
    const onTurnComplete = (currentEvents.on_turn_complete ?? []).filter(
      (a) => !["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
    );
    if (type !== "none") onTurnComplete.push({ type } as OnTurnCompleteAction);
    onUpdate({ events: { ...currentEvents, on_turn_complete: onTurnComplete } });
  };

  const setOnTurnStartTransition = (type: string) => {
    const currentEvents = step.events ?? {};
    const onTurnStart = (currentEvents.on_turn_start ?? []).filter(
      (a) => !["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
    );
    if (type !== "none") onTurnStart.push({ type } as OnTurnStartAction);
    onUpdate({ events: { ...currentEvents, on_turn_start: onTurnStart } });
  };

  const setChildrenCompletedTransition = (type: string, targetStepId?: string) => {
    const currentEvents = step.events ?? {};
    const onChildrenCompleted = (currentEvents.on_children_completed ?? []).filter(
      (a) => !["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
    );
    if (type === "move_to_step") {
      if (!targetStepId) return;
      onChildrenCompleted.push({ type, config: { step_id: targetStepId } } as GenericAction);
    } else if (type !== "none") {
      onChildrenCompleted.push({ type } as GenericAction);
    }
    onUpdate({ events: { ...currentEvents, on_children_completed: onChildrenCompleted } });
  };

  const toggleDisablePlanMode = () => {
    const currentEvents = step.events ?? {};
    const onTurnComplete = currentEvents.on_turn_complete ?? [];
    const exists = onTurnComplete.some((a) => a.type === "disable_plan_mode");
    const newOnTurnComplete = exists
      ? onTurnComplete.filter((a) => a.type !== "disable_plan_mode")
      : [...onTurnComplete, { type: "disable_plan_mode" } as OnTurnCompleteAction];
    onUpdate({ events: { ...currentEvents, on_turn_complete: newOnTurnComplete } });
  };

  const toggleOnExitAction = (type: string) => {
    const currentEvents = step.events ?? {};
    const onExit = currentEvents.on_exit ?? [];
    const exists = onExit.some((a) => a.type === type);
    const newOnExit = exists
      ? onExit.filter((a) => a.type !== type)
      : [...onExit, { type } as OnExitAction];
    onUpdate({ events: { ...currentEvents, on_exit: newOnExit } });
  };

  return {
    toggleOnEnterAction,
    setTransition,
    setOnTurnStartTransition,
    setChildrenCompletedTransition,
    toggleDisablePlanMode,
    toggleOnExitAction,
  };
}

// --- TurnStartSelect ---

type StepSelectProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  otherSteps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

export function TurnStartSelect({
  step,
  savedStep,
  otherSteps,
  onUpdate,
  setOnTurnStartTransition,
  readOnly,
}: StepSelectProps & { setOnTurnStartTransition: (t: string) => void }) {
  const transitionType = getOnTurnStartTransitionType(step);
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Label className="text-xs font-medium">On Turn Start</Label>
        <HelpTip text="Runs when a user sends a message. Use for review cycles (e.g., move back to In Progress on feedback)." />
      </div>
      <Select
        value={transitionType}
        onValueChange={(value) => {
          if (readOnly) return;
          setOnTurnStartTransition(value);
        }}
        disabled={readOnly}
      >
        <SelectTrigger
          className="w-full h-8"
          data-settings-dirty={isWorkflowStepValueDirty(
            step,
            savedStep,
            getOnTurnStartTransitionType,
          )}
        >
          <SelectValue placeholder="Select action" />
        </SelectTrigger>
        <SelectContent position="popper" side="bottom" align="start">
          <SelectItem value="none">Do nothing</SelectItem>
          <SelectItem value="move_to_next">Move to next step</SelectItem>
          <SelectItem value="move_to_previous">Move to previous step</SelectItem>
          <SelectItem value="move_to_step">Move to specific step</SelectItem>
        </SelectContent>
      </Select>
      {transitionType === "move_to_step" && (
        <Select
          value={
            (step.events?.on_turn_start?.find((a) => a.type === "move_to_step")?.config
              ?.step_id as string) ?? ""
          }
          onValueChange={(value) => {
            if (readOnly) return;
            const currentEvents = step.events ?? {};
            const onTurnStart = (currentEvents.on_turn_start ?? []).map((a) =>
              a.type === "move_to_step" ? { ...a, config: { step_id: value } } : a,
            );
            onUpdate({ events: { ...currentEvents, on_turn_start: onTurnStart } });
          }}
          disabled={readOnly}
        >
          <SelectTrigger
            className="w-full h-8"
            data-settings-dirty={isWorkflowStepValueDirty(
              step,
              savedStep,
              (item) =>
                item.events?.on_turn_start?.find((action) => action.type === "move_to_step")?.config
                  ?.step_id ?? "",
            )}
          >
            <SelectValue placeholder="Select step" />
          </SelectTrigger>
          <SelectContent position="popper" side="bottom" align="start">
            {otherSteps.map((s) => (
              <SelectItem key={s.id} value={s.id}>
                <div className="flex items-center gap-2">
                  <div className={cn("w-2 h-2 rounded-full", s.color)} />
                  {s.name}
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}

// --- TurnCompleteSelect ---

type TurnCompleteSelectProps = StepSelectProps & {
  setTransition: (t: string) => void;
  toggleDisablePlanMode: () => void;
  planModeEnabled: boolean;
};

function getTurnCompleteTargetStepId(step: WorkflowStep): string {
  return (
    (step.events?.on_turn_complete?.find((action) => action.type === "move_to_step")?.config
      ?.step_id as string) ?? ""
  );
}

function TurnCompleteTargetSelect({
  step,
  savedStep,
  otherSteps,
  onUpdate,
  readOnly,
}: StepSelectProps) {
  const updateTarget = (stepId: string) => {
    if (readOnly) return;
    const currentEvents = step.events ?? {};
    const onTurnComplete = (currentEvents.on_turn_complete ?? []).map((action) =>
      action.type === "move_to_step" ? { ...action, config: { step_id: stepId } } : action,
    );
    onUpdate({ events: { ...currentEvents, on_turn_complete: onTurnComplete } });
  };

  return (
    <Select
      value={getTurnCompleteTargetStepId(step)}
      onValueChange={updateTarget}
      disabled={readOnly}
    >
      <SelectTrigger
        className="w-full h-8"
        data-settings-dirty={isWorkflowStepValueDirty(step, savedStep, getTurnCompleteTargetStepId)}
      >
        <SelectValue placeholder="Select step" />
      </SelectTrigger>
      <SelectContent position="popper" side="bottom" align="start">
        {otherSteps.map((candidate) => (
          <SelectItem key={candidate.id} value={candidate.id}>
            <div className="flex items-center gap-2">
              <div className={cn("w-2 h-2 rounded-full", candidate.color)} />
              {candidate.name}
            </div>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function TurnCompleteSelect({
  step,
  savedStep,
  otherSteps,
  onUpdate,
  setTransition,
  toggleDisablePlanMode,
  planModeEnabled,
  readOnly,
}: TurnCompleteSelectProps) {
  const transitionType = getTransitionType(step);
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Label className="text-xs font-medium">On Turn Complete</Label>
        <HelpTip text="Runs after the agent finishes a turn. Use to auto-advance tasks through the pipeline." />
      </div>
      <Select
        value={transitionType}
        onValueChange={(value) => {
          if (readOnly) return;
          setTransition(value);
        }}
        disabled={readOnly}
      >
        <SelectTrigger
          className="w-full h-8"
          data-settings-dirty={isWorkflowStepValueDirty(step, savedStep, getTransitionType)}
        >
          <SelectValue placeholder="Select action" />
        </SelectTrigger>
        <SelectContent position="popper" side="bottom" align="start">
          <SelectItem value="none">Do nothing (wait for user)</SelectItem>
          <SelectItem value="move_to_next">Move to next step</SelectItem>
          <SelectItem value="move_to_previous">Move to previous step</SelectItem>
          <SelectItem value="move_to_step">Move to specific step</SelectItem>
        </SelectContent>
      </Select>
      {transitionType === "move_to_step" && (
        <TurnCompleteTargetSelect
          step={step}
          savedStep={savedStep}
          otherSteps={otherSteps}
          onUpdate={onUpdate}
          readOnly={readOnly}
        />
      )}
      {planModeEnabled && (
        <div className="flex items-center gap-2 pt-1">
          <Checkbox
            id={`${step.id}-disable-plan`}
            checked={hasDisablePlanMode(step)}
            onCheckedChange={() => {
              if (readOnly) return;
              toggleDisablePlanMode();
            }}
            disabled={readOnly}
            data-settings-dirty={isWorkflowStepValueDirty(step, savedStep, hasDisablePlanMode)}
          />
          <Label htmlFor={`${step.id}-disable-plan`} className="text-sm">
            Disable plan mode on complete
          </Label>
          <HelpTip text="Turn off plan mode after the agent finishes a turn, even when the task remains in this step." />
        </div>
      )}
      {transitionType !== "none" && (
        <ExplicitCompletionToggle
          step={step}
          savedStep={savedStep}
          onUpdate={onUpdate}
          readOnly={readOnly}
        />
      )}
    </div>
  );
}

// --- ChildrenCompletedSelect ---

type ChildrenCompletedSelectProps = StepSelectProps & {
  setChildrenCompletedTransition: (t: string, targetStepId?: string) => void;
};

export function ChildrenCompletedSelect({
  step,
  savedStep,
  otherSteps,
  onUpdate,
  setChildrenCompletedTransition,
  readOnly,
}: ChildrenCompletedSelectProps) {
  const transitionType = getChildrenCompletedTransitionType(step);
  const configuredTargetStepId =
    (step.events?.on_children_completed?.find((a) => a.type === "move_to_step")?.config
      ?.step_id as string) ?? "";
  const validConfiguredTargetStepId = otherSteps.find((s) => s.id === configuredTargetStepId)?.id;
  const defaultTargetStepId = validConfiguredTargetStepId || otherSteps[0]?.id || "";
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Label className="text-xs font-medium">When Child Tasks Complete</Label>
        <HelpTip
          testId={`${step.id}-children-completed-help`}
          ariaLabel="How child task completion transitions work"
          text="Use this on a parent task step. When every active direct child task is COMPLETED, FAILED, or CANCELLED, Kandev runs this transition once. Archived and ephemeral child tasks are ignored. Grandchildren do not count here, and nothing runs if the parent has no child tasks."
        />
      </div>
      <Select
        value={transitionType}
        onValueChange={(value) => {
          if (readOnly) return;
          setChildrenCompletedTransition(
            value,
            value === "move_to_step" ? defaultTargetStepId : undefined,
          );
        }}
        disabled={readOnly}
      >
        <SelectTrigger
          className="w-full h-8"
          data-testid={`${step.id}-children-completed-transition-select`}
          data-settings-dirty={isWorkflowStepValueDirty(
            step,
            savedStep,
            getChildrenCompletedTransitionType,
          )}
        >
          <SelectValue placeholder="Select action" />
        </SelectTrigger>
        <SelectContent position="popper" side="bottom" align="start">
          <SelectItem value="none">Do nothing</SelectItem>
          <SelectItem value="move_to_next">Move to next step</SelectItem>
          <SelectItem value="move_to_previous">Move to previous step</SelectItem>
          <SelectItem value="move_to_step" disabled={!defaultTargetStepId}>
            Move to specific step
          </SelectItem>
        </SelectContent>
      </Select>
      {transitionType === "move_to_step" && (
        <Select
          value={defaultTargetStepId}
          onValueChange={(value) => {
            if (readOnly) return;
            const currentEvents = step.events ?? {};
            const onChildrenCompleted = (currentEvents.on_children_completed ?? []).map((a) =>
              a.type === "move_to_step" ? { ...a, config: { step_id: value } } : a,
            );
            onUpdate({
              events: { ...currentEvents, on_children_completed: onChildrenCompleted },
            });
          }}
          disabled={readOnly}
        >
          <SelectTrigger
            className="w-full h-8"
            data-testid={`${step.id}-children-completed-step-select`}
            data-settings-dirty={isWorkflowStepValueDirty(
              step,
              savedStep,
              (item) =>
                item.events?.on_children_completed?.find((action) => action.type === "move_to_step")
                  ?.config?.step_id ?? "",
            )}
          >
            <SelectValue placeholder="Select step" />
          </SelectTrigger>
          <SelectContent position="popper" side="bottom" align="start">
            {otherSteps.map((s) => (
              <SelectItem key={s.id} value={s.id}>
                <div className="flex items-center gap-2">
                  <div className={cn("w-2 h-2 rounded-full", s.color)} />
                  {s.name}
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}

// --- ExplicitCompletionToggle ---
// ADR 0015: when checked, on_turn_complete transitions only fire after the
// agent calls the `step_complete_kandev` MCP tool. Bare turn-end leaves the
// task in WAITING_FOR_INPUT until the signal arrives.

type ExplicitCompletionToggleProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

export function ExplicitCompletionToggle({
  step,
  savedStep,
  onUpdate,
  readOnly,
}: ExplicitCompletionToggleProps) {
  return (
    <div className="flex items-center gap-2 pt-1" data-testid={`${step.id}-require-signal-row`}>
      <Checkbox
        id={`${step.id}-require-signal`}
        data-testid={`${step.id}-require-signal-checkbox`}
        checked={step.auto_advance_requires_signal === true}
        onCheckedChange={(c) => {
          if (readOnly) return;
          onUpdate({ auto_advance_requires_signal: c === true });
        }}
        disabled={readOnly}
        data-settings-dirty={isWorkflowStepValueDirty(
          step,
          savedStep,
          (item) => item.auto_advance_requires_signal ?? false,
        )}
      />
      <Label htmlFor={`${step.id}-require-signal`} className="text-sm">
        Wait for agent completion signal
      </Label>
      <HelpTip text="Only auto-advance once the agent calls step_complete_kandev. Otherwise turn-end is treated as completion." />
    </div>
  );
}
