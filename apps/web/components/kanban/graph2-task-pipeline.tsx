'use client';

import { useMemo } from 'react';
import { IconDots, IconTrash } from '@tabler/icons-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { cn } from '@kandev/ui/lib/utils';
import { needsAction } from '@/lib/utils/needs-action';
import { Graph2StepNode } from './graph2-step-node';
import { Graph2Connector } from './graph2-connector';
import type { Task } from '@/components/kanban-card';
import type { WorkflowStep } from '@/components/kanban-column';

type ConnectorType = 'past' | 'transition' | 'future';

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = now - then;
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  return `${months}mo ago`;
}

export type Graph2TaskPipelineProps = {
  task: Task;
  steps: WorkflowStep[];
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  isMoving?: boolean;
  isDeleting?: boolean;
};

export function Graph2TaskPipeline({
  task,
  steps,
  onMoveTask,
  onPreviewTask,
  onEditTask,
  onDeleteTask,
  isMoving,
  isDeleting,
}: Graph2TaskPipelineProps) {
  const currentStepIndex = useMemo(
    () => steps.findIndex((s) => s.id === task.workflowStepId),
    [steps, task.workflowStepId]
  );

  const hasAction = needsAction(task);
  const sessionCount = task.sessionCount ?? 0;

  return (
    <div className="flex items-center justify-center rounded-lg hover:bg-muted/30 transition-colors px-3 py-2">
      <div className="flex items-center gap-3">
        {/* Task name card */}
        <button
          type="button"
          onClick={() => onEditTask(task)}
          className={cn(
            'w-[160px] shrink-0 rounded-md px-2.5 py-1.5 text-left transition-colors cursor-pointer',
            'hover:bg-accent/60 active:bg-accent/80',
            'border border-transparent hover:border-border/50',
            hasAction && 'border-l-2 !border-l-amber-500'
          )}
        >
          <span className="text-xs font-medium truncate block text-foreground/80">
            {task.title}
          </span>
          <div className="flex items-center gap-1.5 mt-0.5">
            {task.updatedAt && (
              <span className="text-[10px] text-muted-foreground/60">
                {formatRelativeTime(task.updatedAt)}
              </span>
            )}
            {sessionCount > 0 && (
              <span className="text-[10px] text-muted-foreground/60">
                {sessionCount} {sessionCount === 1 ? 'session' : 'sessions'}
              </span>
            )}
          </div>
        </button>

        {/* Step nodes with connectors */}
        <div className="flex items-center gap-0">
          {steps.map((step, index) => {
            const phase =
              index < currentStepIndex
                ? 'past'
                : index === currentStepIndex
                  ? 'current'
                  : 'future';

            let connectorType: ConnectorType | null = null;
            if (index < steps.length - 1) {
              const nextPhase =
                index + 1 < currentStepIndex
                  ? 'past'
                  : index + 1 === currentStepIndex
                    ? 'current'
                    : 'future';

              if (phase === 'past' && nextPhase === 'past') {
                connectorType = 'past';
              } else if (phase === 'future' && nextPhase === 'future') {
                connectorType = 'future';
              } else {
                connectorType = 'transition';
              }
            }

            return (
              <div key={step.id} className="flex items-center">
                <Graph2StepNode
                  step={step}
                  phase={phase}
                  task={task}
                  hasPrev={index > 0}
                  hasNext={index < steps.length - 1}
                  prevStepId={index > 0 ? steps[index - 1].id : undefined}
                  nextStepId={index < steps.length - 1 ? steps[index + 1].id : undefined}
                  onMoveTask={onMoveTask}
                  onPreviewTask={onPreviewTask}
                  isMoving={isMoving}
                />
                {connectorType && <Graph2Connector type={connectorType} />}
              </div>
            );
          })}
        </div>

        {/* Actions dropdown */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="shrink-0 h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground/40 hover:text-foreground hover:bg-accent/60 transition-colors cursor-pointer"
            >
              <IconDots className="h-3.5 w-3.5" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-[160px]">
            <DropdownMenuItem
              onClick={() => onDeleteTask(task)}
              disabled={isDeleting}
              className="text-destructive focus:text-destructive cursor-pointer"
            >
              <IconTrash className="h-3.5 w-3.5 mr-2" />
              Delete task
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
