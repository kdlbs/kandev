'use client';

import { useCallback, useMemo, useState } from 'react';
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
  useDroppable,
  useDraggable,
  type Modifier,
} from '@dnd-kit/core';
import { cn } from '@kandev/ui/lib/utils';
import { Badge } from '@kandev/ui/badge';
import { getTaskStateIcon } from '@/lib/ui/state-icons';
import { needsAction } from '@/lib/utils/needs-action';
import { useTaskActions } from '@/hooks/use-task-actions';
import { useAppStoreApi } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Task } from '@/components/kanban-card';
import type { WorkflowStep } from '@/components/kanban-column';
import type { WorkflowAutomation, MoveTaskError } from '@/hooks/use-drag-and-drop';
import type { KanbanState } from '@/lib/state/slices/kanban/types';

export type SwimlaneGraphContentProps = {
  workflowId: string;
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask?: (task: Task) => void;
  onEditTask?: (task: Task) => void;
  onDeleteTask?: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  deletingTaskId?: string | null;
};

function DroppableStepZone({
  stepId,
  isDragging,
  children,
}: {
  stepId: string;
  isDragging: boolean;
  children: React.ReactNode;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: stepId });
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex flex-col items-stretch rounded-lg transition-colors flex-1',
        isDragging && 'border border-dashed border-transparent',
        isDragging && !isOver && 'border-border/40 bg-muted/10',
        isOver && 'border-primary/50 bg-primary/10'
      )}
    >
      {children}
    </div>
  );
}

function DraggableTaskChip({
  task,
  onPreviewTask,
}: {
  task: Task;
  onPreviewTask: (task: Task) => void;
}) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
  });
  const statusIcon = getTaskStateIcon(task.state, 'h-3 w-3');

  return (
    <button
      ref={setNodeRef}
      type="button"
      onClick={() => onPreviewTask(task)}
      {...listeners}
      {...attributes}
      className={cn(
        'w-full flex items-center gap-1.5 px-2 py-1 rounded-md text-left',
        'hover:bg-accent/60 transition-colors cursor-grab',
        'border border-border/50',
        needsAction(task) && 'border-l-2 border-l-amber-500',
        isDragging && 'opacity-30'
      )}
      style={
        transform
          ? { transform: `translate(${transform.x}px, ${transform.y}px)` }
          : undefined
      }
    >
      <div className="shrink-0">{statusIcon}</div>
      <span className="text-xs truncate">{task.title}</span>
    </button>
  );
}

function TaskChipPreview({ task }: { task: Task }) {
  const statusIcon = getTaskStateIcon(task.state, 'h-3 w-3');
  return (
    <div
      className={cn(
        'flex items-center gap-1.5 px-2 py-1 rounded-md',
        'bg-background border border-border shadow-lg cursor-grabbing',
        'pointer-events-none min-w-[120px] max-w-[220px]',
        needsAction(task) && 'border-l-2 border-l-amber-500'
      )}
    >
      <div className="shrink-0">{statusIcon}</div>
      <span className="text-xs truncate">{task.title}</span>
    </div>
  );
}

async function handleWorkflowAutoStart(
  task: Task,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  response: any,
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void,
) {
  const hasAutoStart = response?.workflow_step?.events?.on_enter?.some(
    (a: { type: string }) => a.type === 'auto_start_agent'
  );
  if (!hasAutoStart) return;

  const sessionId = task.primarySessionId ?? null;
  if (sessionId) {
    const client = getWebSocketClient();
    if (!client) return;
    try {
      await client.request(
        'orchestrator.start',
        {
          task_id: task.id,
          session_id: sessionId,
          workflow_step_id: response.workflow_step.id,
        },
        15000
      );
    } catch (err) {
      console.error('Failed to auto-start session for workflow step:', err);
    }
  } else {
    onWorkflowAutomation?.({
      taskId: task.id,
      sessionId: null,
      workflowStep: response.workflow_step,
      taskDescription: task.description ?? '',
    });
  }
}

type SwimlaneGraphDndOptions = {
  tasks: Task[];
  steps: WorkflowStep[];
  workflowId: string;
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  onMoveError?: (error: MoveTaskError) => void;
};

function useSwimlaneGraphDnd({ tasks, steps, workflowId, onWorkflowAutomation, onMoveError }: SwimlaneGraphDndOptions) {
  const store = useAppStoreApi();
  const { moveTaskById } = useTaskActions();
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } })
  );

  const clampVertical: Modifier[] = useMemo(
    () => [({ transform }) => ({ ...transform, y: Math.max(-20, Math.min(20, transform.y)) })],
    []
  );

  const tasksByStep = useMemo(() => {
    const map: Record<string, Task[]> = {};
    for (const col of steps) {
      map[col.id] = tasks.filter((t) => t.workflowStepId === col.id).sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    }
    return map;
  }, [steps, tasks]);

  const adjacentSteps = useMemo(() => {
    const map: Record<string, Set<string>> = {};
    for (let i = 0; i < steps.length; i++) {
      const adj = new Set<string>();
      if (i > 0) adj.add(steps[i - 1].id);
      if (i < steps.length - 1) adj.add(steps[i + 1].id);
      map[steps[i].id] = adj;
    }
    return map;
  }, [steps]);

  const handleDragStart = useCallback((event: DragStartEvent) => { setActiveTaskId(event.active.id as string); }, []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event;
      setActiveTaskId(null);
      if (!over) return;

      const taskId = active.id as string;
      const targetColumnId = over.id as string;
      const task = tasks.find((t) => t.id === taskId);
      if (!task || task.workflowStepId === targetColumnId) return;

      const allowed = adjacentSteps[task.workflowStepId];
      if (!allowed || !allowed.has(targetColumnId)) return;

      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const targetTasks = snapshot.tasks
        .filter((t: KanbanState['tasks'][number]) => t.workflowStepId === targetColumnId && t.id !== taskId)
        .sort((a: KanbanState['tasks'][number], b: KanbanState['tasks'][number]) => a.position - b.position);
      const nextPosition = targetTasks.length;
      const originalTasks = snapshot.tasks;

      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t: KanbanState['tasks'][number]) =>
          t.id === taskId ? { ...t, workflowStepId: targetColumnId, position: nextPosition } : t
        ),
      });

      try {
        const response = await moveTaskById(taskId, { workflow_id: workflowId, workflow_step_id: targetColumnId, position: nextPosition });
        await handleWorkflowAutoStart(task, response, onWorkflowAutomation);
      } catch (error) {
        const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
        if (currentSnapshot) {
          store.getState().setWorkflowSnapshot(workflowId, { ...currentSnapshot, tasks: originalTasks });
        }
        const message = error instanceof Error ? error.message : 'Failed to move task';
        onMoveError?.({ message, taskId, sessionId: task.primarySessionId ?? null });
      }
    },
    [tasks, workflowId, store, moveTaskById, adjacentSteps, onWorkflowAutomation, onMoveError]
  );

  const handleDragCancel = useCallback(() => { setActiveTaskId(null); }, []);
  const activeTask = useMemo(() => tasks.find((t) => t.id === activeTaskId) ?? null, [tasks, activeTaskId]);

  return { sensors, clampVertical, tasksByStep, activeTaskId, handleDragStart, handleDragEnd, handleDragCancel, activeTask };
}

export function SwimlaneGraphContent({
  workflowId, steps, tasks, onPreviewTask, onMoveError, onWorkflowAutomation,
}: SwimlaneGraphContentProps) {
  const { sensors, clampVertical, tasksByStep, activeTaskId, handleDragStart, handleDragEnd, handleDragCancel, activeTask } =
    useSwimlaneGraphDnd({ tasks, steps, workflowId, onWorkflowAutomation, onMoveError });

  if (tasks.length === 0) {
    return (
      <div className="px-3 pb-3">
        <div className="text-xs text-muted-foreground text-center py-4">No tasks</div>
      </div>
    );
  }

  return (
    <DndContext
      sensors={sensors}
      modifiers={clampVertical}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragCancel={handleDragCancel}
    >
      <GraphStepGrid
        steps={steps}
        tasksByStep={tasksByStep}
        isDragging={activeTaskId !== null}
        onPreviewTask={onPreviewTask}
      />
      <DragOverlay dropAnimation={null}>
        {activeTask ? <TaskChipPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}

function GraphStepGrid({
  steps,
  tasksByStep,
  isDragging,
  onPreviewTask,
}: {
  steps: WorkflowStep[];
  tasksByStep: Record<string, Task[]>;
  isDragging: boolean;
  onPreviewTask: (task: Task) => void;
}) {
  return (
    <div className="px-3 pb-3 overflow-x-auto">
      <div className="flex items-stretch justify-center gap-2 min-w-min">
        {steps.map((step, index) => {
          const stepTasks = tasksByStep[step.id] ?? [];
          const hasActiveTasks = stepTasks.length > 0;

          return (
            <div key={step.id} className="flex items-stretch gap-2">
              <DroppableStepZone stepId={step.id} isDragging={isDragging}>
                <div className="w-[240px]">
                  <div
                    className={cn(
                      'rounded-lg border-2 px-3 py-2',
                      hasActiveTasks
                        ? 'border-primary/60 bg-primary/5'
                        : 'border-border bg-muted/30'
                    )}
                  >
                    <div className="flex items-center justify-between gap-1.5">
                      <span className="text-xs font-semibold truncate">{step.title}</span>
                      <Badge
                        variant={hasActiveTasks ? 'default' : 'secondary'}
                        className="text-[10px] shrink-0"
                      >
                        {stepTasks.length}
                      </Badge>
                    </div>
                  </div>

                  {stepTasks.length > 0 && (
                    <div className="mt-2 space-y-1">
                      {stepTasks.map((task) => (
                        <DraggableTaskChip
                          key={task.id}
                          task={task}
                          onPreviewTask={onPreviewTask}
                        />
                      ))}
                    </div>
                  )}
                </div>
              </DroppableStepZone>

              {index < steps.length - 1 && (
                <div className="flex items-center h-[40px] self-start">
                  <div className="w-6 h-px bg-border" />
                  <div className="w-0 h-0 border-t-[4px] border-t-transparent border-b-[4px] border-b-transparent border-l-[6px] border-l-border" />
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
