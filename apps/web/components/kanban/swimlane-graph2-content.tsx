'use client';

import { useMemo, useState } from 'react';
import { useSwimlaneMove } from '@/hooks/domains/kanban/use-swimlane-move';
import { Graph2TaskPipeline } from './graph2-task-pipeline';
import type { ViewContentProps } from '@/lib/kanban/view-registry';

export function SwimlaneGraph2Content({
  workflowId,
  steps,
  tasks,
  onPreviewTask,
  onEditTask,
  onDeleteTask,
  onMoveError,
  onWorkflowAutomation,
  deletingTaskId,
}: ViewContentProps) {
  const { moveTask } = useSwimlaneMove(workflowId, {
    onMoveError,
    onWorkflowAutomation,
  });
  const [movingTaskId, setMovingTaskId] = useState<string | null>(null);

  const sortedTasks = useMemo(
    () =>
      [...tasks].sort((a, b) => {
        const aStepIdx = steps.findIndex((c) => c.id === a.workflowStepId);
        const bStepIdx = steps.findIndex((c) => c.id === b.workflowStepId);
        if (aStepIdx !== bStepIdx) return aStepIdx - bStepIdx;
        return (a.position ?? 0) - (b.position ?? 0);
      }),
    [tasks, steps]
  );

  const handleMoveTask = async (task: typeof tasks[number], targetStepId: string) => {
    setMovingTaskId(task.id);
    try {
      await moveTask(task, targetStepId);
    } finally {
      setMovingTaskId(null);
    }
  };

  if (tasks.length === 0) {
    return (
      <div className="px-3 pb-3">
        <div className="text-xs text-muted-foreground text-center py-4">No tasks</div>
      </div>
    );
  }

  return (
    <div className="px-3 pb-3 overflow-x-auto">
      <div className="space-y-1">
        {sortedTasks.map((task) => (
          <Graph2TaskPipeline
            key={task.id}
            task={task}
            steps={steps}
            onMoveTask={handleMoveTask}
            onPreviewTask={onPreviewTask}
            onEditTask={onEditTask}
            onDeleteTask={onDeleteTask}
            isMoving={movingTaskId === task.id}
            isDeleting={deletingTaskId === task.id}
          />
        ))}
      </div>
    </div>
  );
}
