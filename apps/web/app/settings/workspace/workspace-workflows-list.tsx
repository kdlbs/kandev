"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconGripVertical } from "@tabler/icons-react";
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
  verticalListSortingStrategy,
  arrayMove,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { WorkflowCard } from "@/components/settings/workflow-card";
import { WorkflowListViewToggle } from "@/components/settings/workflow-list-view-toggle";
import { useWorkflowListView } from "@/hooks/domains/settings/use-workflow-list-view";
import { cn } from "@/lib/utils";
import type { Workflow, WorkflowStep } from "@/lib/types/http";

export type WorkflowSavedParams = {
  clientWorkflow: Workflow;
  submittedWorkflow: Workflow;
  savedWorkflow: Workflow;
  currentSteps: WorkflowStep[];
  savedSteps: WorkflowStep[];
  finalizeIdentity: boolean;
};

type WorkflowListProps = {
  workflowItems: Workflow[];
  savedWorkflowItems: Workflow[];
  orderDirtyIds: ReadonlySet<string>;
  initialStepsByWorkflowId: Map<string, WorkflowStep[]>;
  locallyFinalizedWorkflowIds: ReadonlySet<string>;
  isWorkflowDirty: (wf: Workflow) => boolean;
  /** The dedicated Improve Kandev workspace renders workflows read-only. */
  isImproveWorkspace?: boolean;
  onUpdate: (
    id: string,
    u: {
      name?: string;
      description?: string;
      prompt?: string;
      agent_profile_id?: string;
    },
  ) => void;
  onDelete: (id: string) => void;
  onDuplicate: (workflow: Workflow, steps: WorkflowStep[]) => void;
  onWorkflowSaved: (params: WorkflowSavedParams) => void;
  onDiscard: (id: string) => void;
  onReorder: (items: Workflow[]) => void;
};

function SortableWorkflowItem({
  workflow,
  isDirty,
  readOnly,
  compact,
  children,
}: {
  workflow: Workflow;
  isDirty: boolean;
  readOnly?: boolean;
  compact: boolean;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  const {
    attributes,
    listeners,
    setActivatorNodeRef,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: workflow.id, disabled: readOnly });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  return (
    <div
      ref={setNodeRef}
      style={style}
      className="relative min-w-0"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
      data-testid={`workflow-order-item-${workflow.id}`}
    >
      {!readOnly && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                ref={setActivatorNodeRef}
                type="button"
                className={cn(
                  "absolute right-3 z-10 flex h-8 w-8 cursor-grab touch-none items-center justify-center rounded-md text-muted-foreground transition-[color,background-color,transform] hover:bg-muted hover:text-foreground active:scale-95 active:cursor-grabbing max-md:h-11 max-md:w-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11",
                  compact
                    ? "top-1 max-md:top-0 [@media(pointer:coarse)]:top-0"
                    : "top-4 max-md:top-3 [@media(pointer:coarse)]:top-3",
                )}
                data-testid={`workflow-drag-handle-${workflow.id}`}
                {...attributes}
                {...listeners}
                aria-label={t("workflows:reorderWorkflow", { name: workflow.name })}
              >
                <IconGripVertical className="h-5 w-5" aria-hidden="true" />
              </button>
            </TooltipTrigger>
            <TooltipContent>
              {t("workflows:reorderWorkflow", { name: workflow.name })}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
      {children}
    </div>
  );
}

export function WorkflowList({
  workflowItems,
  savedWorkflowItems,
  orderDirtyIds,
  initialStepsByWorkflowId,
  locallyFinalizedWorkflowIds,
  isWorkflowDirty,
  isImproveWorkspace,
  onUpdate,
  onDelete,
  onDuplicate,
  onWorkflowSaved,
  onDiscard,
  onReorder,
}: WorkflowListProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));
  const { view, setView } = useWorkflowListView();
  const compact = view === "compact";
  const savedWorkflowsById = useMemo(
    () => new Map(savedWorkflowItems.map((workflow) => [workflow.id, workflow])),
    [savedWorkflowItems],
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = workflowItems.findIndex((wf) => wf.id === active.id);
    const newIndex = workflowItems.findIndex((wf) => wf.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const reordered = arrayMove(workflowItems, oldIndex, newIndex);
    onReorder(reordered);
  };

  return (
    <div className="grid min-w-0 gap-3">
      <div className="flex justify-end">
        <WorkflowListViewToggle view={view} onViewChange={setView} />
      </div>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext
          items={workflowItems.map((wf) => wf.id)}
          strategy={verticalListSortingStrategy}
        >
          <div
            className="grid min-w-0 gap-3"
            data-settings-dirty={orderDirtyIds.size > 0}
            data-settings-dirty-level="container"
            data-testid="workflow-order-list"
          >
            {workflowItems.map((workflow) => (
              <SortableWorkflowItem
                key={workflow.id}
                workflow={workflow}
                isDirty={orderDirtyIds.has(workflow.id)}
                readOnly={isImproveWorkspace}
                compact={compact}
              >
                <WorkflowCard
                  workflow={workflow}
                  savedWorkflow={savedWorkflowsById.get(workflow.id)}
                  isWorkflowDirty={isWorkflowDirty(workflow)}
                  isOrderDirty={orderDirtyIds.has(workflow.id)}
                  initialWorkflowSteps={initialStepsByWorkflowId.get(workflow.id)}
                  otherWorkflows={workflowItems.filter((w) => w.id !== workflow.id)}
                  isImproveWorkspace={isImproveWorkspace}
                  compact={compact}
                  initiallyExpanded={locallyFinalizedWorkflowIds.has(workflow.id)}
                  onUpdateWorkflow={(updates) => onUpdate(workflow.id, updates)}
                  onDeleteWorkflow={async () => {
                    await onDelete(workflow.id);
                  }}
                  onDuplicateWorkflow={(steps) => onDuplicate(workflow, steps)}
                  onWorkflowSaved={onWorkflowSaved}
                  onDiscardWorkflow={() => onDiscard(workflow.id)}
                />
              </SortableWorkflowItem>
            ))}
          </div>
        </SortableContext>
      </DndContext>
    </div>
  );
}
