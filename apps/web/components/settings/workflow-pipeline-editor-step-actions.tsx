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
import { cn } from "@/lib/utils";
import {
  HelpTip,
  getTransitionType,
  getOnTurnStartTransitionType,
  hasDisablePlanMode,
} from "./workflow-pipeline-editor-helpers";

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
    toggleDisablePlanMode,
    toggleOnExitAction,
  };
}

// --- TurnStartSelect ---

type StepSelectProps = {
  step: WorkflowStep;
  otherSteps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

export function TurnStartSelect({
  step,
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
        <SelectTrigger className="w-full h-8">
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
          <SelectTrigger className="w-full h-8">
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

export function TurnCompleteSelect({
  step,
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
        <SelectTrigger className="w-full h-8">
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
        <Select
          value={
            (step.events?.on_turn_complete?.find((a) => a.type === "move_to_step")?.config
              ?.step_id as string) ?? ""
          }
          onValueChange={(value) => {
            if (readOnly) return;
            const currentEvents = step.events ?? {};
            const onTurnComplete = (currentEvents.on_turn_complete ?? []).map((a) =>
              a.type === "move_to_step" ? { ...a, config: { step_id: value } } : a,
            );
            onUpdate({ events: { ...currentEvents, on_turn_complete: onTurnComplete } });
          }}
          disabled={readOnly}
        >
          <SelectTrigger className="w-full h-8">
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
          />
          <Label htmlFor={`${step.id}-disable-plan`} className="text-sm">
            Disable plan mode on complete
          </Label>
          <HelpTip text="Turn off plan mode after the agent finishes this step." />
        </div>
      )}
    </div>
  );
}
