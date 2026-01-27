'use client';

import { useCallback, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import {
  DndContext,
  closestCenter,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  IconGripVertical,
  IconPlus,
  IconTrash,
  IconChevronDown,
  IconChevronUp,
  IconRobot,
  IconCheck,
  IconClipboard,
} from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Textarea } from '@kandev/ui/textarea';
import { Checkbox } from '@kandev/ui/checkbox';
import { Label } from '@kandev/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import type { WorkflowStep, WorkflowStepType, StepBehaviors } from '@/lib/types/http';

// Debounce hook for text inputs
function useDebouncedCallback(
  callback: (value: string) => void,
  delay: number
): (value: string) => void {
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  return useCallback(
    function debouncedFn(value: string) {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
      timeoutRef.current = setTimeout(() => {
        callback(value);
      }, delay);
    },
    [callback, delay]
  );
}

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

const STEP_TYPES: { value: WorkflowStepType; label: string }[] = [
  { value: 'backlog', label: 'Backlog' },
  { value: 'planning', label: 'Planning' },
  { value: 'implementation', label: 'Implementation' },
  { value: 'review', label: 'Review' },
  { value: 'verification', label: 'Verification' },
  { value: 'done', label: 'Done' },
  { value: 'blocked', label: 'Blocked' },
];

type WorkflowStepEditorProps = {
  steps: WorkflowStep[];
  onUpdateStep: (stepId: string, updates: Partial<WorkflowStep>) => void;
  onAddStep: () => void;
  onRemoveStep: (stepId: string) => void;
  onReorderSteps: (steps: WorkflowStep[]) => void;
};

type SortableStepCardProps = {
  step: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove: () => void;
};

// Inner component that uses step values directly - reset via key prop from parent
function SortableStepCardInner({ step, onUpdate, onRemove }: SortableStepCardProps) {
  const [expanded, setExpanded] = useState(false);
  // Local state for text inputs to avoid API calls on every keystroke
  const [localName, setLocalName] = useState(step.name);
  const [localPromptPrefix, setLocalPromptPrefix] = useState(step.behaviors?.promptPrefix ?? '');
  const [localPromptSuffix, setLocalPromptSuffix] = useState(step.behaviors?.promptSuffix ?? '');

  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({
    id: step.id,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  const behaviors = step.behaviors ?? {};

  // Debounced update functions
  const debouncedUpdateName = useDebouncedCallback((name: string) => {
    onUpdate({ name });
  }, 500);

  const debouncedUpdatePromptPrefix = useDebouncedCallback((promptPrefix: string) => {
    onUpdate({ behaviors: { ...behaviors, promptPrefix } });
  }, 500);

  const debouncedUpdatePromptSuffix = useDebouncedCallback((promptSuffix: string) => {
    onUpdate({ behaviors: { ...behaviors, promptSuffix } });
  }, 500);

  const updateBehaviors = (updates: Partial<StepBehaviors>) => {
    onUpdate({ behaviors: { ...behaviors, ...updates } });
  };

  return (
    <Card ref={setNodeRef} style={style} className="mb-2">
      <CardContent className="p-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="p-1 rounded-md text-muted-foreground hover:text-foreground cursor-grab"
            {...attributes}
            {...listeners}
          >
            <IconGripVertical className="h-4 w-4" />
          </button>

          <div className={`w-4 h-4 rounded ${step.color}`} />

          <Input
            value={localName}
            onChange={(e) => {
              setLocalName(e.target.value);
              debouncedUpdateName(e.target.value);
            }}
            className="flex-1"
            placeholder="Step name"
          />

          <Select
            value={step.step_type}
            onValueChange={(value: WorkflowStepType) => onUpdate({ step_type: value })}
          >
            <SelectTrigger className="w-[140px]">
              <SelectValue placeholder="Type" />
            </SelectTrigger>
            <SelectContent>
              {STEP_TYPES.map((type) => (
                <SelectItem key={type.value} value={type.value}>
                  {type.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="flex items-center gap-1 text-muted-foreground">
            {behaviors.autoStartAgent && <IconRobot className="h-4 w-4" title="Auto-start agent" />}
            {behaviors.requireApproval && <IconCheck className="h-4 w-4" title="Require approval" />}
            {behaviors.planMode && <IconClipboard className="h-4 w-4" title="Plan mode" />}
          </div>

          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? <IconChevronUp className="h-4 w-4" /> : <IconChevronDown className="h-4 w-4" />}
          </Button>

          <Button type="button" variant="ghost" size="icon-sm" onClick={onRemove}>
            <IconTrash className="h-4 w-4" />
          </Button>
        </div>

        {expanded && (
          <div className="mt-4 pt-4 border-t space-y-4">
            <div className="grid grid-cols-3 gap-4">
              <div className="flex items-center space-x-2">
                <Checkbox
                  id={`${step.id}-auto-start`}
                  checked={behaviors.autoStartAgent ?? false}
                  onCheckedChange={(checked) => updateBehaviors({ autoStartAgent: checked === true })}
                />
                <Label htmlFor={`${step.id}-auto-start`}>Auto-start agent</Label>
              </div>
              <div className="flex items-center space-x-2">
                <Checkbox
                  id={`${step.id}-plan-mode`}
                  checked={behaviors.planMode ?? false}
                  onCheckedChange={(checked) => updateBehaviors({ planMode: checked === true })}
                />
                <Label htmlFor={`${step.id}-plan-mode`}>Plan mode</Label>
              </div>
              <div className="flex items-center space-x-2">
                <Checkbox
                  id={`${step.id}-require-approval`}
                  checked={behaviors.requireApproval ?? false}
                  onCheckedChange={(checked) => updateBehaviors({ requireApproval: checked === true })}
                />
                <Label htmlFor={`${step.id}-require-approval`}>Require approval</Label>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor={`${step.id}-prompt-prefix`}>Prompt Prefix</Label>
              <Textarea
                id={`${step.id}-prompt-prefix`}
                value={localPromptPrefix}
                onChange={(e) => {
                  setLocalPromptPrefix(e.target.value);
                  debouncedUpdatePromptPrefix(e.target.value);
                }}
                placeholder="Text prepended to the task description when agent starts..."
                rows={3}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor={`${step.id}-prompt-suffix`}>Prompt Suffix</Label>
              <Textarea
                id={`${step.id}-prompt-suffix`}
                value={localPromptSuffix}
                onChange={(e) => {
                  setLocalPromptSuffix(e.target.value);
                  debouncedUpdatePromptSuffix(e.target.value);
                }}
                placeholder="Text appended to the task description when agent starts..."
                rows={3}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor={`${step.id}-color`}>Color</Label>
              <Select value={step.color} onValueChange={(value) => onUpdate({ color: value })}>
                <SelectTrigger className="w-[180px]">
                  <div className="flex items-center gap-2">
                    <div className={`w-3 h-3 rounded ${step.color}`} />
                    <SelectValue placeholder="Select color" />
                  </div>
                </SelectTrigger>
                <SelectContent>
                  {STEP_COLORS.map((color) => (
                    <SelectItem key={color.value} value={color.value}>
                      <div className="flex items-center gap-2">
                        <div className={`w-3 h-3 rounded ${color.value}`} />
                        {color.label}
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// Wrapper component that uses key to reset local state when step data changes externally
function SortableStepCard({ step, onUpdate, onRemove }: SortableStepCardProps) {
  // Generate a stable key based on step values that should trigger a reset
  // This resets local state when the step is reordered or updated externally
  const resetKey = `${step.id}-${step.name}-${step.behaviors?.promptPrefix ?? ''}-${step.behaviors?.promptSuffix ?? ''}`;
  return <SortableStepCardInner key={resetKey} step={step} onUpdate={onUpdate} onRemove={onRemove} />;
}

export function WorkflowStepEditor({
  steps,
  onUpdateStep,
  onAddStep,
  onRemoveStep,
  onReorderSteps,
}: WorkflowStepEditorProps) {
  const stepItems = useMemo(() => steps.map((step) => step.id), [steps]);
  const isMounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false
  );

  const handleDragEnd = (event: DragEndEvent) => {
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

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium">Workflow Steps</h3>
        <Button variant="outline" size="sm" onClick={onAddStep}>
          <IconPlus className="mr-2 h-4 w-4" />
          Add Step
        </Button>
      </div>

      <div className="space-y-2">
        {isMounted ? (
          <DndContext collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={stepItems}>
              {steps.map((step) => (
                <SortableStepCard
                  key={step.id}
                  step={step}
                  onUpdate={(updates) => onUpdateStep(step.id, updates)}
                  onRemove={() => onRemoveStep(step.id)}
                />
              ))}
            </SortableContext>
          </DndContext>
        ) : (
          steps.map((step) => (
            <SortableStepCard
              key={step.id}
              step={step}
              onUpdate={(updates) => onUpdateStep(step.id, updates)}
              onRemove={() => onRemoveStep(step.id)}
            />
          ))
        )}
      </div>
    </div>
  );
}

