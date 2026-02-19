"use client";

import { useState } from "react";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { WorkflowStep } from "@/lib/types/http";
import { useDebouncedCallback } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import {
  HelpTip,
  STEP_COLORS,
  PROMPT_TEMPLATES,
  hasOnEnterAction,
  hasOnExitAction,
} from "./workflow-pipeline-editor-helpers";
import {
  useStepActions,
  TurnStartSelect,
  TurnCompleteSelect,
} from "./workflow-pipeline-editor-step-actions";

// --- StepConfigHeader ---

type StepConfigHeaderProps = {
  step: WorkflowStep;
  localName: string;
  onLocalNameChange: (name: string) => void;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove: () => void;
  readOnly: boolean;
  debouncedUpdateName: (name: string) => void;
};

function StepConfigHeader({
  step,
  localName,
  onLocalNameChange,
  onUpdate,
  onRemove,
  readOnly,
  debouncedUpdateName,
}: StepConfigHeaderProps) {
  return (
    <div className="flex items-center justify-between px-4 py-3 border-b border-border">
      <div className="flex items-center gap-3 flex-1 min-w-0">
        <Input
          id={`${step.id}-name`}
          value={localName}
          onChange={(e) => {
            if (readOnly) return;
            onLocalNameChange(e.target.value);
            debouncedUpdateName(e.target.value);
          }}
          placeholder="Step name"
          disabled={readOnly}
          className="max-w-[240px] h-8"
        />
        <Select
          value={step.color}
          onValueChange={(value) => {
            if (readOnly) return;
            onUpdate({ color: value });
          }}
          disabled={readOnly}
        >
          <SelectTrigger className="w-[120px] h-8">
            <SelectValue placeholder="Color" />
          </SelectTrigger>
          <SelectContent position="popper" side="bottom" align="start">
            {STEP_COLORS.map((color) => (
              <SelectItem key={color.value} value={color.value}>
                <div className="flex items-center gap-2">
                  <div className={cn("w-3 h-3 rounded-full", color.value)} />
                  {color.label}
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={onRemove}
        disabled={readOnly}
        className="text-destructive hover:text-destructive h-8"
      >
        <IconTrash className="h-3.5 w-3.5 mr-1" />
        Delete
      </Button>
    </div>
  );
}

// --- StepAutoArchiveRow ---

type StepAutoArchiveRowProps = {
  step: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

function StepAutoArchiveRow({ step, onUpdate, readOnly }: StepAutoArchiveRowProps) {
  return (
    <div className="flex items-center gap-2 pt-1">
      <Checkbox
        id={`${step.id}-auto-archive`}
        checked={(step.auto_archive_after_hours ?? 0) > 0}
        onCheckedChange={(checked) => {
          if (readOnly) return;
          onUpdate({ auto_archive_after_hours: checked ? 24 : 0 });
        }}
        disabled={readOnly}
      />
      <Label htmlFor={`${step.id}-auto-archive`} className="text-sm">
        Auto-archive
      </Label>
      <HelpTip text="Automatically archive tasks after they have been in this step for a set number of hours. Useful for the last step of a workflow (e.g., Done) to keep the board clean." />
      {(step.auto_archive_after_hours ?? 0) > 0 && (
        <>
          <span className="text-sm text-muted-foreground">after</span>
          <Input
            id={`${step.id}-auto-archive-hours`}
            type="number"
            min={1}
            className="w-20 h-7 text-sm"
            value={step.auto_archive_after_hours ?? 24}
            onChange={(e) => {
              if (readOnly) return;
              const val = parseInt(e.target.value, 10);
              onUpdate({ auto_archive_after_hours: isNaN(val) || val < 1 ? 1 : val });
            }}
            disabled={readOnly}
          />
          <span className="text-sm text-muted-foreground">hours</span>
        </>
      )}
    </div>
  );
}

// --- StepBehaviorSection ---

type StepBehaviorSectionProps = {
  step: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  toggleOnEnterAction: (type: string) => void;
  readOnly: boolean;
};

function StepBehaviorSection({
  step,
  onUpdate,
  toggleOnEnterAction,
  readOnly,
}: StepBehaviorSectionProps) {
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
        <div className="flex items-center gap-2">
          <Checkbox
            id={`${step.id}-start-step`}
            checked={step.is_start_step === true}
            onCheckedChange={(checked) => {
              if (readOnly) return;
              onUpdate({ is_start_step: checked === true });
            }}
            disabled={readOnly}
          />
          <Label htmlFor={`${step.id}-start-step`} className="text-sm">
            Start step
          </Label>
          <HelpTip text="New tasks start in this step. Only one step per workflow can be the start step." />
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id={`${step.id}-auto-start`}
            checked={hasOnEnterAction(step, "auto_start_agent")}
            onCheckedChange={() => {
              if (readOnly) return;
              toggleOnEnterAction("auto_start_agent");
            }}
            disabled={readOnly}
          />
          <Label htmlFor={`${step.id}-auto-start`} className="text-sm">
            Auto-start agent
          </Label>
          <HelpTip text="Automatically start the agent when a task enters this step." />
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id={`${step.id}-plan-mode`}
            checked={hasOnEnterAction(step, "enable_plan_mode")}
            onCheckedChange={() => {
              if (readOnly) return;
              toggleOnEnterAction("enable_plan_mode");
            }}
            disabled={readOnly}
          />
          <Label htmlFor={`${step.id}-plan-mode`} className="text-sm">
            Plan mode
          </Label>
          <HelpTip text="Agent proposes a plan instead of making changes directly." />
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id={`${step.id}-manual-move`}
            checked={step.allow_manual_move !== false}
            onCheckedChange={(checked) => {
              if (readOnly) return;
              onUpdate({ allow_manual_move: checked === true });
            }}
            disabled={readOnly}
          />
          <Label htmlFor={`${step.id}-manual-move`} className="text-sm">
            Allow manual move
          </Label>
          <HelpTip text="Allow dragging tasks into this step on the board." />
        </div>
      </div>
      <StepAutoArchiveRow step={step} onUpdate={onUpdate} readOnly={readOnly} />
    </div>
  );
}

// --- StepTransitionsSection ---

type StepTransitionsSectionProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  setTransition: (type: string) => void;
  setOnTurnStartTransition: (type: string) => void;
  toggleDisablePlanMode: () => void;
  toggleOnExitAction: (type: string) => void;
  readOnly: boolean;
};

function StepTransitionsSection({
  step,
  steps,
  onUpdate,
  setTransition,
  setOnTurnStartTransition,
  toggleDisablePlanMode,
  toggleOnExitAction,
  readOnly,
}: StepTransitionsSectionProps) {
  const otherSteps = steps.filter((s) => s.id !== step.id);
  const planModeEnabled = hasOnEnterAction(step, "enable_plan_mode");

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-1.5">
        <Label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          Transitions
        </Label>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <TurnStartSelect
          step={step}
          otherSteps={otherSteps}
          onUpdate={onUpdate}
          setOnTurnStartTransition={setOnTurnStartTransition}
          readOnly={readOnly}
        />
        <TurnCompleteSelect
          step={step}
          otherSteps={otherSteps}
          onUpdate={onUpdate}
          setTransition={setTransition}
          toggleDisablePlanMode={toggleDisablePlanMode}
          planModeEnabled={planModeEnabled}
          readOnly={readOnly}
        />
      </div>
      {planModeEnabled && (
        <div className="space-y-2">
          <div className="flex items-center gap-1.5">
            <Label className="text-xs font-medium">On Exit</Label>
            <HelpTip text="Runs when leaving this step (before entering the next step)." />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id={`${step.id}-exit-disable-plan`}
              checked={hasOnExitAction(step, "disable_plan_mode")}
              onCheckedChange={() => {
                if (readOnly) return;
                toggleOnExitAction("disable_plan_mode");
              }}
              disabled={readOnly}
            />
            <Label htmlFor={`${step.id}-exit-disable-plan`} className="text-sm">
              Disable plan mode
            </Label>
            <HelpTip text="Turn off plan mode when leaving this step." />
          </div>
        </div>
      )}
    </div>
  );
}

// --- StepPromptSection ---

type StepPromptSectionProps = {
  step: WorkflowStep;
  localPrompt: string;
  onLocalPromptChange: (prompt: string) => void;
  debouncedUpdatePrompt: (prompt: string) => void;
  readOnly: boolean;
};

function StepPromptSection({
  step,
  localPrompt,
  onLocalPromptChange,
  debouncedUpdatePrompt,
  readOnly,
}: StepPromptSectionProps) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Label
          htmlFor={`${step.id}-prompt`}
          className="text-xs font-medium text-muted-foreground uppercase tracking-wider"
        >
          Step Prompt
        </Label>
        <HelpTip text="Custom instructions for the agent on this step. Use {{task_prompt}} to include the task description." />
      </div>
      {!readOnly && (
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className="text-[11px] text-muted-foreground/60">Templates:</span>
          {PROMPT_TEMPLATES.map((template) => (
            <button
              key={template.label}
              type="button"
              onClick={() => {
                onLocalPromptChange(template.prompt);
                debouncedUpdatePrompt(template.prompt);
              }}
              className="text-[11px] px-2 py-0.5 rounded-md border border-border bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors cursor-pointer"
            >
              {template.label}
            </button>
          ))}
        </div>
      )}
      <Textarea
        id={`${step.id}-prompt`}
        value={localPrompt}
        onChange={(e) => {
          if (readOnly) return;
          onLocalPromptChange(e.target.value);
          debouncedUpdatePrompt(e.target.value);
        }}
        placeholder={
          "Instructions for the agent on this step.\nUse {{task_prompt}} to include the task description."
        }
        rows={3}
        disabled={readOnly}
        className="font-mono text-xs max-h-[200px] overflow-y-auto resize-y"
      />
      <p className="text-[11px] text-muted-foreground/60">
        If set, this prompt will be used instead of the task description. Use{" "}
        <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{task_prompt}}"}</code> to
        include the original task description within it.
      </p>
    </div>
  );
}

// --- StepConfigPanel ---

type StepConfigPanelProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove: () => void;
  readOnly?: boolean;
};

export function StepConfigPanel({
  step,
  steps,
  onUpdate,
  onRemove,
  readOnly = false,
}: StepConfigPanelProps) {
  const [localName, setLocalName] = useState(step.name);
  const [localPrompt, setLocalPrompt] = useState(step.prompt ?? "");

  const debouncedUpdateName = useDebouncedCallback((name: string) => {
    onUpdate({ name });
  }, 500);
  const debouncedUpdatePrompt = useDebouncedCallback((prompt: string) => {
    onUpdate({ prompt });
  }, 500);

  const actions = useStepActions({ step, onUpdate });

  return (
    <div
      key={step.id}
      className="rounded-lg border bg-card animate-in fade-in-0 slide-in-from-top-2 duration-200"
    >
      <StepConfigHeader
        step={step}
        localName={localName}
        onLocalNameChange={setLocalName}
        onUpdate={onUpdate}
        onRemove={onRemove}
        readOnly={readOnly}
        debouncedUpdateName={debouncedUpdateName}
      />
      <div className="p-4 space-y-5">
        <StepBehaviorSection
          step={step}
          onUpdate={onUpdate}
          toggleOnEnterAction={actions.toggleOnEnterAction}
          readOnly={readOnly}
        />
        <StepTransitionsSection
          step={step}
          steps={steps}
          onUpdate={onUpdate}
          setTransition={actions.setTransition}
          setOnTurnStartTransition={actions.setOnTurnStartTransition}
          toggleDisablePlanMode={actions.toggleDisablePlanMode}
          toggleOnExitAction={actions.toggleOnExitAction}
          readOnly={readOnly}
        />
        <StepPromptSection
          step={step}
          localPrompt={localPrompt}
          onLocalPromptChange={setLocalPrompt}
          debouncedUpdatePrompt={debouncedUpdatePrompt}
          readOnly={readOnly}
        />
      </div>
    </div>
  );
}
