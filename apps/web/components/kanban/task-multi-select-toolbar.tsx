"use client";

import { useMemo, useState } from "react";
import { IconTrash, IconArchive, IconChevronRight, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import type { WorkflowStep } from "@/components/kanban-column";

interface TaskMultiSelectToolbarProps {
  selectedIds: Set<string>;
  steps: WorkflowStep[];
  isProcessing: boolean;
  canMove?: boolean;
  onClearSelection: () => void;
  onBulkDelete: (opts?: { cascade?: boolean }) => Promise<void>;
  onBulkArchive: (opts?: { cascade?: boolean }) => Promise<void>;
  onBulkMove: (targetStepId: string) => Promise<void>;
}

function useBulkExecutorTypes(taskIds: string[]): Array<string | null | undefined> {
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const fallbackTasks = useAppStore((state) => state.kanban.tasks);
  return useMemo(
    () =>
      taskIds.map(
        (id) => findTaskInSnapshots(id, snapshots, fallbackTasks)?.primaryExecutorType ?? null,
      ),
    [taskIds, snapshots, fallbackTasks],
  );
}

function BulkArchiveDialog({
  count,
  taskIds,
  isProcessing,
  onConfirm,
}: {
  count: number;
  taskIds: string[];
  isProcessing: boolean;
  onConfirm: (opts: { cascade: boolean }) => void;
}) {
  const [open, setOpen] = useState(false);
  const executorTypes = useBulkExecutorTypes(taskIds);

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        className="cursor-pointer gap-1.5"
        disabled={isProcessing}
        onClick={() => setOpen(true)}
        data-testid="bulk-archive-button"
      >
        <IconArchive className="h-4 w-4" />
        Archive {count}
      </Button>
      <TaskArchiveConfirmDialog
        open={open}
        onOpenChange={setOpen}
        isBulkOperation
        count={count}
        taskIds={taskIds}
        executorTypes={executorTypes}
        isArchiving={isProcessing}
        onConfirm={onConfirm}
        confirmTestId="bulk-archive-confirm"
      />
    </>
  );
}

function BulkDeleteDialog({
  count,
  taskIds,
  isProcessing,
  onConfirm,
}: {
  count: number;
  taskIds: string[];
  isProcessing: boolean;
  onConfirm: (opts: { cascade: boolean }) => void;
}) {
  const [open, setOpen] = useState(false);
  const executorTypes = useBulkExecutorTypes(taskIds);

  return (
    <>
      <Button
        size="sm"
        variant="destructive"
        className="cursor-pointer gap-1.5"
        disabled={isProcessing}
        onClick={() => setOpen(true)}
        data-testid="bulk-delete-button"
      >
        <IconTrash className="h-4 w-4" />
        Delete {count}
      </Button>
      <TaskDeleteConfirmDialog
        open={open}
        onOpenChange={setOpen}
        isBulkOperation
        count={count}
        taskIds={taskIds}
        executorTypes={executorTypes}
        isDeleting={isProcessing}
        onConfirm={onConfirm}
        confirmTestId="bulk-delete-confirm"
      />
    </>
  );
}

export function TaskMultiSelectToolbar({
  selectedIds,
  steps,
  isProcessing,
  canMove = true,
  onClearSelection,
  onBulkDelete,
  onBulkArchive,
  onBulkMove,
}: TaskMultiSelectToolbarProps) {
  if (selectedIds.size === 0) return null;

  const count = selectedIds.size;

  return (
    <div
      className={cn(
        "fixed bottom-6 left-1/2 -translate-x-1/2 z-50",
        "flex items-center gap-2 px-4 py-2 rounded-xl shadow-lg border border-border",
        "bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/75",
      )}
      data-testid="multi-select-toolbar"
    >
      <span className="text-sm font-medium text-muted-foreground mr-1">{count} selected</span>

      {steps.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="sm"
              variant="outline"
              className="cursor-pointer gap-1.5"
              disabled={isProcessing || !canMove}
              title={!canMove ? "Cannot move tasks from different workflows" : undefined}
              data-testid="bulk-move-button"
            >
              Move to
              <IconChevronRight className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="center" side="top">
            {steps.map((step) => (
              <DropdownMenuItem
                key={step.id}
                className="cursor-pointer"
                onClick={() => onBulkMove(step.id)}
                data-testid={`bulk-move-step-${step.id}`}
              >
                <div className={cn("w-2 h-2 rounded-full mr-2 shrink-0", step.color)} />
                {step.title}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}

      <BulkArchiveDialog
        count={count}
        taskIds={[...selectedIds]}
        isProcessing={isProcessing}
        onConfirm={({ cascade }) => onBulkArchive({ cascade })}
      />

      <BulkDeleteDialog
        count={count}
        taskIds={[...selectedIds]}
        isProcessing={isProcessing}
        onConfirm={({ cascade }) => onBulkDelete({ cascade })}
      />

      <Button
        size="sm"
        variant="ghost"
        className="cursor-pointer ml-1"
        onClick={onClearSelection}
        disabled={isProcessing}
        aria-label="Clear selection"
        data-testid="bulk-clear-selection"
      >
        <IconX className="h-4 w-4" />
      </Button>
    </div>
  );
}
