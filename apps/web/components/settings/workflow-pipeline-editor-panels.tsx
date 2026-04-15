"use client";

import { useState } from "react";
import { IconRobot, IconTrash } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { WorkflowStep } from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import { useDebouncedCallback } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";
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

const STEP_PROMPT_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "task_prompt",
    description: "The original task description provided by the user",
    example: "Implement user authentication with OAuth2",
    executor_types: [],
  },
];

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
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const healthyProfiles = agentProfiles.filter(
    (p) =>
      !p.capability_status || p.capability_status === "ok" || p.capability_status === "probing",
  );

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
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex items-center">
                <Select
                  value={step.agent_profile_id || "none"}
                  onValueChange={(value) => {
                    if (readOnly) return;
                    onUpdate({ agent_profile_id: value === "none" ? "" : value });
                  }}
                  disabled={readOnly}
                >
                  <SelectTrigger
                    className="w-[220px] h-8 cursor-pointer"
                    data-testid="step-agent-profile-select"
                  >
                    <IconRobot className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <SelectValue placeholder="No profile override" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none" className="cursor-pointer">
                      No profile override
                    </SelectItem>
                    {healthyProfiles.map((p) => (
                      <SelectItem key={p.id} value={p.id} className="cursor-pointer">
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">
              Override the agent profile for this step. A different profile creates a new session
              with fresh context when entering this step.
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
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
    <div className="flex items-center gap-2">
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

// --- StepCheckboxRow ---

type StepCheckboxRowProps = {
  id: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled: boolean;
  label: string;
  helpText: string;
};

function StepCheckboxRow({
  id,
  checked,
  onCheckedChange,
  disabled,
  label,
  helpText,
}: StepCheckboxRowProps) {
  return (
    <div className="flex items-center gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        disabled={disabled}
      />
      <Label htmlFor={id} className="text-sm">
        {label}
      </Label>
      <HelpTip text={helpText} />
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
        <StepCheckboxRow
          id={`${step.id}-start-step`}
          checked={step.is_start_step === true}
          onCheckedChange={(c) => !readOnly && onUpdate({ is_start_step: c })}
          disabled={readOnly}
          label="Start step"
          helpText="New tasks start in this step. Only one step per workflow can be the start step."
        />
        <StepCheckboxRow
          id={`${step.id}-auto-start`}
          checked={hasOnEnterAction(step, "auto_start_agent")}
          onCheckedChange={() => !readOnly && toggleOnEnterAction("auto_start_agent")}
          disabled={readOnly}
          label="Auto-start agent"
          helpText="Automatically start the agent when a task enters this step."
        />
        <StepCheckboxRow
          id={`${step.id}-plan-mode`}
          checked={hasOnEnterAction(step, "enable_plan_mode")}
          onCheckedChange={() => !readOnly && toggleOnEnterAction("enable_plan_mode")}
          disabled={readOnly}
          label="Plan mode"
          helpText="Agent proposes a plan instead of making changes directly."
        />
        <StepCheckboxRow
          id={`${step.id}-reset-context`}
          checked={hasOnEnterAction(step, "reset_agent_context")}
          onCheckedChange={() => !readOnly && toggleOnEnterAction("reset_agent_context")}
          disabled={readOnly}
          label="Reset agent context"
          helpText="Restart the agent with a fresh conversation context when entering this step. Useful for review steps that need an unbiased perspective."
        />
        <StepCheckboxRow
          id={`${step.id}-manual-move`}
          checked={step.allow_manual_move !== false}
          onCheckedChange={(c) => !readOnly && onUpdate({ allow_manual_move: c })}
          disabled={readOnly}
          label="Allow manual move"
          helpText="Allow dragging tasks into this step on the board."
        />
        <StepCheckboxRow
          id={`${step.id}-command-panel`}
          checked={step.show_in_command_panel !== false}
          onCheckedChange={(c) => !readOnly && onUpdate({ show_in_command_panel: c })}
          disabled={readOnly}
          label="Show in command panel"
          helpText="Show tasks in this step when opening the command panel (Cmd+K). Useful for hiding backlog or done steps from quick access."
        />
        <StepAutoArchiveRow step={step} onUpdate={onUpdate} readOnly={readOnly} />
      </div>
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
      <div className="rounded-md border overflow-hidden">
        <ScriptEditor
          value={localPrompt}
          onChange={(v) => {
            if (readOnly) return;
            onLocalPromptChange(v);
            debouncedUpdatePrompt(v);
          }}
          language="markdown"
          height={computeEditorHeight(localPrompt)}
          lineNumbers="off"
          readOnly={readOnly}
          placeholders={STEP_PROMPT_PLACEHOLDERS}
        />
      </div>
      <p className="text-[11px] text-muted-foreground/60">
        Type {"{{"} to see available placeholders. Use{" "}
        <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{"{{task_prompt}}"}</code> to
        include the original task description.
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
