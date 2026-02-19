"use client";

import { useState } from "react";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type {
  WorkflowStep,
  OnEnterAction,
  OnTurnStartAction,
  OnTurnCompleteAction,
  OnExitAction,
} from "@/lib/types/http";
import { useDebouncedCallback } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import { HelpTip } from "./workflow-pipeline-editor-helpers";

const STEP_COLORS = [
  { value: "bg-slate-500", label: "Gray" },
  { value: "bg-red-500", label: "Red" },
  { value: "bg-orange-500", label: "Orange" },
  { value: "bg-yellow-500", label: "Yellow" },
  { value: "bg-green-500", label: "Green" },
  { value: "bg-cyan-500", label: "Cyan" },
  { value: "bg-blue-500", label: "Blue" },
  { value: "bg-indigo-500", label: "Indigo" },
  { value: "bg-purple-500", label: "Purple" },
];

const PROMPT_TEMPLATES = [
  {
    label: "Plan",
    prompt: `Analyze the task and create a detailed implementation plan.

{{task_prompt}}

INSTRUCTIONS:
1. Break the task into clear, ordered steps
2. For each step, describe what needs to be done and which files are affected
3. Identify potential risks or blockers
4. Estimate relative complexity for each step (low/medium/high)

Output the plan as a numbered list. Be specific about file paths, function names, and the approach for each step. Do NOT implement anything yet — only plan.`,
  },
  {
    label: "Code Review",
    prompt: `Please review the changed files in the current git worktree.

STEP 1: Determine what to review
- First, check if there are any uncommitted changes (dirty working directory)
- If there are uncommitted/staged changes: review those files
- If the working directory is clean: review the commits that have diverged from the master/main branch

STEP 2: Review the changes, then output your findings in EXACTLY 4 sections: BUG, IMPROVEMENT, NITPICK, PERFORMANCE.

Rules:
- Each section is OPTIONAL — only include it if you have findings for that category
- If a section has no findings, omit it entirely
- Format each finding as: filename:line_number - Description
- Be specific and reference exact line numbers
- Keep descriptions concise but actionable
- Sort findings by severity within each section
- Focus on logic and design issues, NOT formatting or style that automated tools handle

Section definitions:

BUG: Critical issues that will cause runtime errors, crashes, incorrect behavior, data corruption, or logic errors
- Examples: null/nil dereference, race conditions, incorrect algorithms, type mismatches, resource leaks, deadlocks

IMPROVEMENT: Code quality, architecture, security, or maintainability concerns
- Examples: missing error handling, incorrect access modifiers, SQL injection vulnerabilities, hardcoded credentials, tight coupling, missing validation

NITPICK: Significant readability or maintainability issues that impact code understanding
- Examples: misleading variable/function names, overly complex logic that should be refactored, missing critical comments for complex algorithms
- EXCLUDE: formatting, whitespace, import ordering, trivial naming preferences, style issues handled by linters/formatters

PERFORMANCE: Algorithmic or resource usage problems with measurable impact
- Examples: O(n²) where O(n) or O(1) is possible, unnecessary allocations in loops, missing indexes for database queries, blocking I/O in hot paths
- Concurrency-specific: unprotected shared state, missing synchronization, goroutine leaks, missing context cancellation

Now review the changes.`,
  },
  {
    label: "Security Audit",
    prompt: `Perform a security audit on the changed files in the current git worktree.

{{task_prompt}}

Review all changes and check for the following categories:

1. **Injection Vulnerabilities**: SQL injection, command injection, XSS, template injection, path traversal
2. **Authentication & Authorization**: Missing auth checks, broken access control, privilege escalation, insecure session handling
3. **Data Exposure**: Hardcoded secrets, credentials in logs, sensitive data in error messages, missing encryption
4. **Input Validation**: Missing or insufficient validation at system boundaries, unsafe deserialization, unrestricted file uploads
5. **Dependency Risks**: Known vulnerable dependencies, unsafe use of third-party libraries
6. **Concurrency Issues**: Race conditions on shared state, TOCTOU bugs, unsafe concurrent access to resources

For each finding, output:
- **Severity**: CRITICAL / HIGH / MEDIUM / LOW
- **Location**: filename:line_number
- **Issue**: What the vulnerability is
- **Impact**: What an attacker could do
- **Fix**: Specific remediation steps

Only report real, actionable findings. Do not flag theoretical issues without evidence in the code.`,
  },
];

// --- Helpers ---

function hasOnEnterAction(step: WorkflowStep, type: string): boolean {
  return step.events?.on_enter?.some((a) => a.type === type) ?? false;
}

function getTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_turn_complete?.find((a) =>
    ["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
  );
  return action?.type ?? "none";
}

function getOnTurnStartTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_turn_start?.find((a) =>
    ["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
  );
  return action?.type ?? "none";
}

function hasDisablePlanMode(step: WorkflowStep): boolean {
  return step.events?.on_turn_complete?.some((a) => a.type === "disable_plan_mode") ?? false;
}

function hasOnExitAction(step: WorkflowStep, type: string): boolean {
  return step.events?.on_exit?.some((a) => a.type === type) ?? false;
}

// --- useStepActions hook ---

type UseStepActionsArgs = {
  step: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
};

function useStepActions({ step, onUpdate }: UseStepActionsArgs) {
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

type StepSelectProps = {
  step: WorkflowStep;
  otherSteps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

function TurnStartSelect({
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

type TurnCompleteSelectProps = StepSelectProps & {
  setTransition: (t: string) => void;
  toggleDisablePlanMode: () => void;
  planModeEnabled: boolean;
};

function TurnCompleteSelect({
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
