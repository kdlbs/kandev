"use client";

import { useMemo, useState, useSyncExternalStore } from "react";
import {
  DndContext,
  closestCenter,
  type DragEndEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  horizontalListSortingStrategy,
  arrayMove,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  IconGripVertical,
  IconPlus,
  IconTrash,
  IconChevronRight,
  IconRosetteNumber1,
} from "@tabler/icons-react";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { ScrollArea, ScrollBar } from "@kandev/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import type { WorkflowStep } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { StepConfigPanel } from "./workflow-pipeline-editor-panels";

type WorkflowPipelineEditorProps = {
  steps: WorkflowStep[];
  onUpdateStep: (stepId: string, updates: Partial<WorkflowStep>) => void;
  onAddStep: () => void;
  onRemoveStep: (stepId: string) => void;
  onReorderSteps: (steps: WorkflowStep[]) => void;
  readOnly?: boolean;
};

// --- Helpers ---

function getTransitionType(step: WorkflowStep): string {
  const action = step.events?.on_turn_complete?.find((a) =>
    ["move_to_next", "move_to_previous", "move_to_step"].includes(a.type),
  );
  return action?.type ?? "none";
}

function getTransitionLabel(step: WorkflowStep): string {
  const t = getTransitionType(step);
  if (t === "move_to_next") return "auto";
  if (t === "move_to_previous") return "back";
  if (t === "move_to_step") return "goto";
  return "manual";
}

// --- Pipeline Node ---

type PipelineNodeProps = {
  step: WorkflowStep;
  isSelected: boolean;
  onSelect: () => void;
  onRemove: () => void;
  readOnly?: boolean;
};

function PipelineNode({
  step,
  isSelected,
  onSelect,
  onRemove,
  readOnly = false,
}: PipelineNodeProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: step.id,
  });

  const style = { transform: CSS.Transform.toString(transform), transition };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        "group relative flex items-center gap-1.5 rounded-lg border-2 px-3 py-2 min-w-[120px] max-w-[160px] cursor-pointer transition-colors select-none",
        isSelected
          ? "border-primary bg-primary/5"
          : "border-border bg-card hover:border-primary/50",
        isDragging && "opacity-50 z-50",
      )}
      onClick={onSelect}
    >
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
      <button
        type="button"
        className={cn(
          "shrink-0 p-0.5 rounded text-muted-foreground/40 hover:text-muted-foreground",
          readOnly ? "cursor-default" : "cursor-grab",
        )}
        {...(readOnly ? {} : attributes)}
        {...(readOnly ? {} : listeners)}
        aria-disabled={readOnly}
        onClick={(e) => e.stopPropagation()}
      >
        <IconGripVertical className="h-3.5 w-3.5" />
      </button>
      <div className="flex flex-col gap-0.5 min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <div className={cn("w-3 h-3 rounded-full shrink-0", step.color)} />
          <span className="text-sm font-medium truncate">{step.name}</span>
        </div>
        <StepCapabilityIcons
          events={step.events}
          fallback={<span className="text-xs text-muted-foreground/50">manual</span>}
        />
      </div>
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

// --- Pipeline Area ---

type PipelineAreaProps = {
  steps: WorkflowStep[];
  selectedStepId: string | null;
  onSelectStep: (stepId: string) => void;
  onRemoveStep: (stepId: string) => void;
  onAddStep: () => void;
  readOnly: boolean;
};

function PipelineArea({
  steps,
  selectedStepId,
  onSelectStep,
  onRemoveStep,
  onAddStep,
  readOnly,
}: PipelineAreaProps) {
  return (
    <div className="flex items-center gap-0 py-2 px-1">
      {steps.map((step, index) => (
        <div key={step.id} className="flex items-center">
          {index > 0 && <PipelineConnector label={getTransitionLabel(steps[index - 1])} />}
          <PipelineNode
            step={step}
            isSelected={selectedStepId === step.id}
            onSelect={() => onSelectStep(step.id)}
            onRemove={() => onRemoveStep(step.id)}
            readOnly={readOnly}
          />
        </div>
      ))}
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

  if (steps.length !== prevStepCount) {
    if (steps.length > prevStepCount && steps.length > 0)
      setSelectedStepId(steps[steps.length - 1].id);
    setPrevStepCount(steps.length);
  }

  const stepItems = useMemo(() => steps.map((step) => step.id), [steps]);
  const isMounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false,
  );

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));

  const handleDragEnd = (event: DragEndEvent) => {
    if (readOnly) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = steps.findIndex((step) => step.id === active.id);
    const newIndex = steps.findIndex((step) => step.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    onReorderSteps(
      arrayMove(steps, oldIndex, newIndex).map((step, index) => ({ ...step, position: index })),
    );
  };

  const handleSelectStep = (stepId: string) =>
    setSelectedStepId((prev) => (prev === stepId ? null : stepId));

  const handleRemoveStep = (stepId: string) => {
    onRemoveStep(stepId);
    if (selectedStepId === stepId) setSelectedStepId(null);
  };

  const selectedStep = steps.find((s) => s.id === selectedStepId);
  const pipelineArea = (
    <PipelineArea
      steps={steps}
      selectedStepId={selectedStepId}
      onSelectStep={handleSelectStep}
      onRemoveStep={handleRemoveStep}
      onAddStep={onAddStep}
      readOnly={readOnly}
    />
  );

  return (
    <div className="space-y-3">
      <ScrollArea className="w-full">
        {isMounted ? (
          <DndContext
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
            sensors={sensors}
          >
            <SortableContext items={stepItems} strategy={horizontalListSortingStrategy}>
              {pipelineArea}
            </SortableContext>
          </DndContext>
        ) : (
          pipelineArea
        )}
        <ScrollBar orientation="horizontal" />
      </ScrollArea>
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
