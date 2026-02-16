'use client';

import { useMemo, useState, useSyncExternalStore } from 'react';
import {
  DndContext,
  closestCenter,
  type DragEndEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  SortableContext,
  horizontalListSortingStrategy,
  arrayMove,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  IconGripVertical,
  IconPlus,
  IconTrash,
  IconChevronRight,
  IconInfoCircle,
  IconRosetteNumber1,
} from '@tabler/icons-react';
import { StepCapabilityIcons } from '@/components/step-capability-icons';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Textarea } from '@kandev/ui/textarea';
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import { ScrollArea, ScrollBar } from '@kandev/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@kandev/ui/tooltip';
import type { WorkflowStep, OnEnterAction, OnTurnStartAction, OnTurnCompleteAction } from '@/lib/types/http';
import { useDebouncedCallback } from '@/hooks/use-debounce';
import { cn } from '@/lib/utils';

const STEP_COLORS = [
  { value: 'bg-slate-500', label: 'Gray' },
  { value: 'bg-red-500', label: 'Red' },
  { value: 'bg-orange-500', label: 'Orange' },
  { value: 'bg-yellow-500', label: 'Yellow' },
  { value: 'bg-green-500', label: 'Green' },
  { value: 'bg-cyan-500', label: 'Cyan' },
  { value: 'bg-blue-500', label: 'Blue' },
  { value: 'bg-indigo-500', label: 'Indigo' },
  { value: 'bg-purple-500', label: 'Purple' },
];

type WorkflowPipelineEditorProps = {
  steps: WorkflowStep[];
  onUpdateStep: (stepId: string, updates: Partial<WorkflowStep>) => void;
  onAddStep: () => void;
  onRemoveStep: (stepId: string) => void;
  onReorderSteps: (steps: WorkflowStep[]) => void;
  readOnly?: boolean;
};

// --- Helpers ---

function hasOnEnterAction(step: WorkflowStep, type: string): boolean {
  return step.events?.on_enter?.some((a) => a.type === type) ?? false;
}

function getTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_turn_complete?.find((a) =>
    ['move_to_next', 'move_to_previous', 'move_to_step'].includes(a.type)
  );
  return action?.type ?? 'none';
}

function getOnTurnStartTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_turn_start?.find((a) =>
    ['move_to_next', 'move_to_previous', 'move_to_step'].includes(a.type)
  );
  return action?.type ?? 'none';
}

function hasDisablePlanMode(step: WorkflowStep): boolean {
  return step.events?.on_turn_complete?.some((a) => a.type === 'disable_plan_mode') ?? false;
}

function getTransitionLabel(step: WorkflowStep): string {
  const t = getTransitionType(step);
  if (t === 'move_to_next') return 'auto';
  if (t === 'move_to_previous') return 'back';
  if (t === 'move_to_step') return 'goto';
  return 'manual';
}

// --- Prompt Templates ---

const PROMPT_TEMPLATES = [
  {
    label: 'Plan',
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
    label: 'Code Review',
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
- Examples: O(n\u00B2) where O(n) or O(1) is possible, unnecessary allocations in loops, missing indexes for database queries, blocking I/O in hot paths
- Concurrency-specific: unprotected shared state, missing synchronization, goroutine leaks, missing context cancellation

Now review the changes.`,
  },
  {
    label: 'Security Audit',
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

// --- Inline help tooltip ---

function HelpTip({ text }: { text: string }) {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">{text}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

// --- Pipeline Node ---

type PipelineNodeProps = {
  step: WorkflowStep;
  isSelected: boolean;
  onSelect: () => void;
  onRemove: () => void;
  readOnly?: boolean;
};

function PipelineNode({ step, isSelected, onSelect, onRemove, readOnly = false }: PipelineNodeProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: step.id,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'group relative flex items-center gap-1.5 rounded-lg border-2 px-3 py-2 min-w-[120px] max-w-[160px] cursor-pointer transition-colors select-none',
        isSelected
          ? 'border-primary bg-primary/5'
          : 'border-border bg-card hover:border-primary/50',
        isDragging && 'opacity-50 z-50',
      )}
      onClick={onSelect}
    >
      {/* Start step indicator */}
      {step.is_start_step && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="absolute -top-2 -left-2 flex items-center justify-center w-5 h-5 rounded-full bg-amber-500 text-white">
                <IconRosetteNumber1 className="h-3.5 w-3.5" />
              </div>
            </TooltipTrigger>
            <TooltipContent>Start step</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}

      {/* Drag handle */}
      <button
        type="button"
        className={cn(
          'shrink-0 p-0.5 rounded text-muted-foreground/40 hover:text-muted-foreground',
          readOnly ? 'cursor-default' : 'cursor-grab',
        )}
        {...(readOnly ? {} : attributes)}
        {...(readOnly ? {} : listeners)}
        aria-disabled={readOnly}
        onClick={(e) => e.stopPropagation()}
      >
        <IconGripVertical className="h-3.5 w-3.5" />
      </button>

      {/* Content */}
      <div className="flex flex-col gap-0.5 min-w-0 flex-1">
        {/* Color dot + Name */}
        <div className="flex items-center gap-1.5">
          <div className={cn('w-3 h-3 rounded-full shrink-0', step.color)} />
          <span className="text-sm font-medium truncate">{step.name}</span>
        </div>

        {/* Event icons */}
        <StepCapabilityIcons events={step.events} fallback={
          <span className="text-xs text-muted-foreground/50">manual</span>
        } />
      </div>

      {/* Delete button on hover */}
      {!readOnly && (
        <button
          type="button"
          className="absolute -top-2 -right-2 hidden group-hover:flex items-center justify-center w-5 h-5 rounded-full bg-destructive text-destructive-foreground hover:bg-destructive/90 cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
        >
          <IconTrash className="h-3 w-3" />
        </button>
      )}
    </div>
  );
}

// --- Connector ---

function PipelineConnector({ label }: { label: string }) {
  return (
    <div className="flex flex-col items-center justify-center shrink-0 px-1">
      <div className="flex items-center gap-0.5 text-muted-foreground/60">
        <div className="w-4 h-px bg-border" />
        <IconChevronRight className="h-3 w-3" />
      </div>
      <span className="text-[10px] text-muted-foreground/50 leading-none mt-0.5">{label}</span>
    </div>
  );
}

// --- Step Config Panel ---

type StepConfigPanelProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove: () => void;
  readOnly?: boolean;
};

function StepConfigPanel({ step, steps, onUpdate, onRemove, readOnly = false }: StepConfigPanelProps) {
  const [localName, setLocalName] = useState(step.name);
  const [localPrompt, setLocalPrompt] = useState(step.prompt ?? '');

  const debouncedUpdateName = useDebouncedCallback((name: string) => {
    onUpdate({ name });
  }, 500);

  const debouncedUpdatePrompt = useDebouncedCallback((prompt: string) => {
    onUpdate({ prompt });
  }, 500);

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
      (a) => !['move_to_next', 'move_to_previous', 'move_to_step'].includes(a.type)
    );
    if (type !== 'none') {
      onTurnComplete.push({ type } as OnTurnCompleteAction);
    }
    onUpdate({ events: { ...currentEvents, on_turn_complete: onTurnComplete } });
  };

  const setOnTurnStartTransition = (type: string) => {
    const currentEvents = step.events ?? {};
    const onTurnStart = (currentEvents.on_turn_start ?? []).filter(
      (a) => !['move_to_next', 'move_to_previous', 'move_to_step'].includes(a.type)
    );
    if (type !== 'none') {
      onTurnStart.push({ type } as OnTurnStartAction);
    }
    onUpdate({ events: { ...currentEvents, on_turn_start: onTurnStart } });
  };

  const toggleDisablePlanMode = () => {
    const currentEvents = step.events ?? {};
    const onTurnComplete = currentEvents.on_turn_complete ?? [];
    const exists = onTurnComplete.some((a) => a.type === 'disable_plan_mode');
    const newOnTurnComplete = exists
      ? onTurnComplete.filter((a) => a.type !== 'disable_plan_mode')
      : [...onTurnComplete, { type: 'disable_plan_mode' } as OnTurnCompleteAction];
    onUpdate({ events: { ...currentEvents, on_turn_complete: newOnTurnComplete } });
  };

  // Reset local state only when the step identity changes (not on local edits round-tripping)
  const resetKey = step.id;

  return (
    <div key={resetKey} className="rounded-lg border bg-card animate-in fade-in-0 slide-in-from-top-2 duration-200">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <Input
            id={`${step.id}-name`}
            value={localName}
            onChange={(e) => {
              if (readOnly) return;
              setLocalName(e.target.value);
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
                    <div className={cn('w-3 h-3 rounded-full', color.value)} />
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

      {/* Body */}
      <div className="p-4 space-y-5">
        {/* Behavior section */}
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
              <Label htmlFor={`${step.id}-start-step`} className="text-sm">Start step</Label>
              <HelpTip text="New tasks start in this step. Only one step per workflow can be the start step." />
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id={`${step.id}-auto-start`}
                checked={hasOnEnterAction(step, 'auto_start_agent')}
                onCheckedChange={() => {
                  if (readOnly) return;
                  toggleOnEnterAction('auto_start_agent');
                }}
                disabled={readOnly}
              />
              <Label htmlFor={`${step.id}-auto-start`} className="text-sm">Auto-start agent</Label>
              <HelpTip text="Automatically start the agent when a task enters this step." />
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id={`${step.id}-plan-mode`}
                checked={hasOnEnterAction(step, 'enable_plan_mode')}
                onCheckedChange={() => {
                  if (readOnly) return;
                  toggleOnEnterAction('enable_plan_mode');
                }}
                disabled={readOnly}
              />
              <Label htmlFor={`${step.id}-plan-mode`} className="text-sm">Plan mode</Label>
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
              <Label htmlFor={`${step.id}-manual-move`} className="text-sm">Allow manual move</Label>
              <HelpTip text="Allow dragging tasks into this step on the board." />
            </div>
          </div>
          {/* Auto-archive setting */}
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
            <Label htmlFor={`${step.id}-auto-archive`} className="text-sm">Auto-archive</Label>
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

        {/* Transitions section */}
        <div className="space-y-3">
          <div className="flex items-center gap-1.5">
            <Label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Transitions</Label>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            {/* On Turn Start */}
            <div className="space-y-2">
              <div className="flex items-center gap-1.5">
                <Label className="text-xs font-medium">On Turn Start</Label>
                <HelpTip text="Runs when a user sends a message. Use for review cycles (e.g., move back to In Progress on feedback)." />
              </div>
              <Select
                value={getOnTurnStartTransitionType(step)}
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

              {getOnTurnStartTransitionType(step) === 'move_to_step' && (
                <Select
                  value={
                    step.events?.on_turn_start?.find((a) => a.type === 'move_to_step')?.config?.step_id as string ?? ''
                  }
                  onValueChange={(value) => {
                    if (readOnly) return;
                    const currentEvents = step.events ?? {};
                    const onTurnStart = (currentEvents.on_turn_start ?? []).map((a) =>
                      a.type === 'move_to_step' ? { ...a, config: { step_id: value } } : a
                    );
                    onUpdate({ events: { ...currentEvents, on_turn_start: onTurnStart } });
                  }}
                  disabled={readOnly}
                >
                  <SelectTrigger className="w-full h-8">
                    <SelectValue placeholder="Select step" />
                  </SelectTrigger>
                  <SelectContent position="popper" side="bottom" align="start">
                    {steps
                      .filter((s) => s.id !== step.id)
                      .map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          <div className="flex items-center gap-2">
                            <div className={cn('w-2 h-2 rounded-full', s.color)} />
                            {s.name}
                          </div>
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              )}
            </div>

            {/* On Turn Complete */}
            <div className="space-y-2">
              <div className="flex items-center gap-1.5">
                <Label className="text-xs font-medium">On Turn Complete</Label>
                <HelpTip text="Runs after the agent finishes a turn. Use to auto-advance tasks through the pipeline." />
              </div>
              <Select
                value={getTransitionType(step)}
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

              {getTransitionType(step) === 'move_to_step' && (
                <Select
                  value={
                    step.events?.on_turn_complete?.find((a) => a.type === 'move_to_step')?.config?.step_id as string ?? ''
                  }
                  onValueChange={(value) => {
                    if (readOnly) return;
                    const currentEvents = step.events ?? {};
                    const onTurnComplete = (currentEvents.on_turn_complete ?? []).map((a) =>
                      a.type === 'move_to_step' ? { ...a, config: { step_id: value } } : a
                    );
                    onUpdate({ events: { ...currentEvents, on_turn_complete: onTurnComplete } });
                  }}
                  disabled={readOnly}
                >
                  <SelectTrigger className="w-full h-8">
                    <SelectValue placeholder="Select step" />
                  </SelectTrigger>
                  <SelectContent position="popper" side="bottom" align="start">
                    {steps
                      .filter((s) => s.id !== step.id)
                      .map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          <div className="flex items-center gap-2">
                            <div className={cn('w-2 h-2 rounded-full', s.color)} />
                            {s.name}
                          </div>
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              )}

              {hasOnEnterAction(step, 'enable_plan_mode') && (
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
                  <Label htmlFor={`${step.id}-disable-plan`} className="text-sm">Disable plan mode on complete</Label>
                  <HelpTip text="Turn off plan mode after the agent finishes this step." />
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Prompt section */}
        <div className="space-y-2">
          <div className="flex items-center gap-1.5">
            <Label htmlFor={`${step.id}-prompt`} className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Step Prompt</Label>
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
                    setLocalPrompt(template.prompt);
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
              setLocalPrompt(e.target.value);
              debouncedUpdatePrompt(e.target.value);
            }}
            placeholder={'Instructions for the agent on this step.\nUse {{task_prompt}} to include the task description.'}
            rows={3}
            disabled={readOnly}
            className="font-mono text-xs max-h-[200px] overflow-y-auto resize-y"
          />
          <p className="text-[11px] text-muted-foreground/60">
            If set, this prompt will be used instead of the task description. Use <code className="bg-muted px-1 py-0.5 rounded text-[10px]">{'{{task_prompt}}'}</code> to include the original task description within it.
          </p>
        </div>
      </div>
    </div>
  );
}

// --- Main Pipeline Editor ---

export function WorkflowPipelineEditor({
  steps,
  onUpdateStep,
  onAddStep,
  onRemoveStep,
  onReorderSteps,
  readOnly = false,
}: WorkflowPipelineEditorProps) {
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);
  const [prevStepCount, setPrevStepCount] = useState(steps.length);

  // Auto-select newly added step (render-time state adjustment)
  if (steps.length !== prevStepCount) {
    if (steps.length > prevStepCount && steps.length > 0) {
      setSelectedStepId(steps[steps.length - 1].id);
    }
    setPrevStepCount(steps.length);
  }

  const stepItems = useMemo(() => steps.map((step) => step.id), [steps]);
  const isMounted = useSyncExternalStore(
    () => () => { },
    () => true,
    () => false,
  );

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
  );

  const handleDragEnd = (event: DragEndEvent) => {
    if (readOnly) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = steps.findIndex((step) => step.id === active.id);
    const newIndex = steps.findIndex((step) => step.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const nextSteps = arrayMove(steps, oldIndex, newIndex).map((step, index) => ({
      ...step,
      position: index,
    }));
    onReorderSteps(nextSteps);
  };

  const selectedStep = steps.find((s) => s.id === selectedStepId);

  const handleSelectStep = (stepId: string) => {
    setSelectedStepId((prev) => (prev === stepId ? null : stepId));
  };

  const pipelineContent = (
    <div className="flex items-center gap-0 py-2 px-1">
      {steps.map((step, index) => (
        <div key={step.id} className="flex items-center">
          {index > 0 && <PipelineConnector label={getTransitionLabel(steps[index - 1])} />}
          <PipelineNode
            step={step}
            isSelected={selectedStepId === step.id}
            onSelect={() => handleSelectStep(step.id)}
            onRemove={() => {
              onRemoveStep(step.id);
              if (selectedStepId === step.id) setSelectedStepId(null);
            }}
            readOnly={readOnly}
          />
        </div>
      ))}

      {/* Add step button */}
      {steps.length > 0 && (
        <div className="flex items-center">
          <div className="w-4 h-px bg-border shrink-0" />
        </div>
      )}
      <button
        type="button"
        onClick={readOnly ? undefined : onAddStep}
        disabled={readOnly}
        className="shrink-0 h-10 w-10 rounded-lg border border-dashed border-border text-muted-foreground hover:border-primary/50 hover:text-foreground flex items-center justify-center cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        <IconPlus className="h-4 w-4" />
      </button>
    </div>
  );

  return (
    <div className="space-y-3">
      {/* Horizontal scrollable pipeline */}
      <ScrollArea className="w-full">
        {isMounted ? (
          <DndContext collisionDetection={closestCenter} onDragEnd={handleDragEnd} sensors={sensors}>
            <SortableContext items={stepItems} strategy={horizontalListSortingStrategy}>
              {pipelineContent}
            </SortableContext>
          </DndContext>
        ) : (
          pipelineContent
        )}
        <ScrollBar orientation="horizontal" />
      </ScrollArea>

      {/* Config panel for selected step */}
      {selectedStep && (
        <StepConfigPanel
          key={selectedStep.id}
          step={selectedStep}
          steps={steps}
          onUpdate={(updates) => onUpdateStep(selectedStep.id, updates)}
          onRemove={() => {
            onRemoveStep(selectedStep.id);
            setSelectedStepId(null);
          }}
          readOnly={readOnly}
        />
      )}
    </div>
  );
}
