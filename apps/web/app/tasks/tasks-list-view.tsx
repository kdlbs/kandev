"use client";

import { useMemo, useState } from "react";
import type { PaginationState } from "@tanstack/react-table";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconArchive,
  IconChevronLeft,
  IconChevronRight,
  IconLoader,
  IconPlus,
  IconSearch,
  IconTrash,
} from "@tabler/icons-react";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import {
  primaryTaskRepository,
  type Repository,
  type Task,
  type Workflow,
  type WorkflowStep,
} from "@/lib/types/http";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import { formatRelativeTime } from "@/lib/utils";

export type TasksListViewProps = {
  total: number;
  showArchived: boolean;
  setShowArchived: (show: boolean) => void;
  tasks: Task[];
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  pageCount: number;
  pagination: PaginationState;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  isLoading: boolean;
  handleRowClick: (task: Task) => void;
  searchQuery: string;
  setSearchQuery: (query: string) => void;
  setCreateDialogOpen: (open: boolean) => void;
  deletingTaskId: string | null;
  handleArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  handleDelete: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
};

export function TasksListView({
  total,
  showArchived,
  setShowArchived,
  tasks,
  workflows,
  steps,
  repositories,
  pageCount,
  pagination,
  setPagination,
  isLoading,
  handleRowClick,
  searchQuery,
  setSearchQuery,
  setCreateDialogOpen,
  deletingTaskId,
  handleArchive,
  handleDelete,
}: TasksListViewProps) {
  return (
    <main className="flex-1 overflow-auto px-4 py-4 sm:px-6 sm:py-6">
      <div className="space-y-4">
        <TasksListToolbar
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          showArchived={showArchived}
          onShowArchivedChange={setShowArchived}
          onNewTask={() => setCreateDialogOpen(true)}
        />
        <TaskRows
          tasks={tasks}
          workflows={workflows}
          steps={steps}
          repositories={repositories}
          isLoading={isLoading}
          deletingTaskId={deletingTaskId}
          onArchive={handleArchive}
          onDelete={handleDelete}
          onRowClick={handleRowClick}
        />
        <TasksPagination
          total={total}
          pageCount={pageCount}
          pagination={pagination}
          onPaginationChange={setPagination}
        />
      </div>
    </main>
  );
}

function TasksListToolbar({
  searchQuery,
  onSearchChange,
  showArchived,
  onShowArchivedChange,
  onNewTask,
}: {
  searchQuery: string;
  onSearchChange: (query: string) => void;
  showArchived: boolean;
  onShowArchivedChange: (show: boolean) => void;
  onNewTask: () => void;
}) {
  return (
    <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
      <Button className="h-11 w-full cursor-pointer sm:w-auto lg:h-9" onClick={onNewTask}>
        <IconPlus className="mr-1 h-4 w-4" />
        New Task
      </Button>
      <div className="relative w-full lg:max-w-[400px]">
        <IconSearch className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={searchQuery}
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder="Search tasks..."
          className="h-11 pl-9 text-[16px] lg:h-9 lg:text-sm"
        />
      </div>
      <Label className="flex h-11 items-center gap-2 text-sm text-muted-foreground cursor-pointer select-none lg:h-9">
        <Checkbox
          checked={showArchived}
          onCheckedChange={(checked) => onShowArchivedChange(checked === true)}
          className="cursor-pointer"
        />
        Show archived
      </Label>
    </div>
  );
}

function TaskRows({
  tasks,
  workflows,
  steps,
  repositories,
  isLoading,
  deletingTaskId,
  onArchive,
  onDelete,
  onRowClick,
}: {
  tasks: Task[];
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  isLoading: boolean;
  deletingTaskId: string | null;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onDelete: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onRowClick: (task: Task) => void;
}) {
  const workflowMap = useMemo(() => new Map(workflows.map((w) => [w.id, w.name])), [workflows]);
  const stepMap = useMemo(() => new Map(steps.map((s) => [s.id, s.name])), [steps]);
  const repoMap = useMemo(() => new Map(repositories.map((r) => [r.id, r.name])), [repositories]);

  if (isLoading) {
    return (
      <div className="rounded-lg border border-border p-8 text-center text-sm text-muted-foreground">
        Loading tasks...
      </div>
    );
  }
  if (tasks.length === 0) {
    return (
      <div className="rounded-lg border border-border p-8 text-center text-sm text-muted-foreground">
        No tasks found.
      </div>
    );
  }

  return (
    <div
      className="rounded-lg border border-border divide-y divide-border"
      data-testid="tasks-list"
    >
      {tasks.map((task) => (
        <TaskListRow
          key={task.id}
          task={task}
          workflowName={workflowMap.get(task.workflow_id)}
          stepName={stepMap.get(task.workflow_step_id)}
          repositoryName={resolveRepositoryName(task, repoMap)}
          deletingTaskId={deletingTaskId}
          onArchive={onArchive}
          onDelete={onDelete}
          onRowClick={onRowClick}
        />
      ))}
    </div>
  );
}

function resolveRepositoryName(task: Task, repoMap: Map<string, string>): string | undefined {
  const primaryRepo = primaryTaskRepository(task.repositories);
  return primaryRepo ? repoMap.get(primaryRepo.repository_id) : undefined;
}

function TaskListRow({
  task,
  workflowName,
  stepName,
  repositoryName,
  deletingTaskId,
  onArchive,
  onDelete,
  onRowClick,
}: {
  task: Task;
  workflowName?: string;
  stepName?: string;
  repositoryName?: string;
  deletingTaskId: string | null;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onDelete: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onRowClick: (task: Task) => void;
}) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const isDeleting = deletingTaskId === task.id;
  const isArchived = !!task.archived_at;

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="tasks-list-row"
      className="grid min-h-[56px] grid-cols-1 gap-3 px-4 py-3 text-sm transition-colors hover:bg-muted/60 cursor-pointer md:grid-cols-[minmax(0,1fr)_auto] md:items-center"
      onClick={() => onRowClick(task)}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return;
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onRowClick(task);
        }
      }}
    >
      <div className="flex min-w-0 items-center gap-2">
        {getTaskStateIcon(task.state, "h-4 w-4 shrink-0")}
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <span className="min-w-0 truncate font-medium">{task.title}</span>
            {isArchived && (
              <Badge
                variant="outline"
                className="shrink-0 border-amber-500/30 px-1.5 py-0 text-[10px] text-amber-500"
              >
                Archived
              </Badge>
            )}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>{formatTaskStateLabel(task.state)}</span>
            {repositoryName && <span className="font-mono">{repositoryName}</span>}
            <span>{workflowName ?? "-"}</span>
            <span className="rounded-md bg-foreground/[0.06] px-2 py-0.5">{stepName ?? "-"}</span>
          </div>
        </div>
      </div>
      <div className="flex items-center justify-between gap-3 md:justify-end">
        <span className="text-xs text-muted-foreground">{formatRelativeTime(task.updated_at)}</span>
        <TaskRowActions
          task={task}
          isArchived={isArchived}
          isDeleting={isDeleting}
          showDeleteConfirm={showDeleteConfirm}
          showArchiveConfirm={showArchiveConfirm}
          onDeleteOpenChange={setShowDeleteConfirm}
          onArchiveOpenChange={setShowArchiveConfirm}
          onArchive={onArchive}
          onDelete={onDelete}
        />
      </div>
    </div>
  );
}

function TaskRowActions({
  task,
  isArchived,
  isDeleting,
  showDeleteConfirm,
  showArchiveConfirm,
  onDeleteOpenChange,
  onArchiveOpenChange,
  onArchive,
  onDelete,
}: {
  task: Task;
  isArchived: boolean;
  isDeleting: boolean;
  showDeleteConfirm: boolean;
  showArchiveConfirm: boolean;
  onDeleteOpenChange: (open: boolean) => void;
  onArchiveOpenChange: (open: boolean) => void;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onDelete: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
}) {
  return (
    <div className="flex items-center gap-1" onClick={(event) => event.stopPropagation()}>
      {!isArchived && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-9 w-9 cursor-pointer"
              onClick={() => onArchiveOpenChange(true)}
            >
              <IconArchive className="h-4 w-4 text-muted-foreground" />
              <span className="sr-only">Archive task</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>Archive</TooltipContent>
        </Tooltip>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-9 w-9 cursor-pointer"
            disabled={isDeleting}
            onClick={() => onDeleteOpenChange(true)}
          >
            {isDeleting ? (
              <IconLoader className="h-4 w-4 animate-spin" />
            ) : (
              <IconTrash className="h-4 w-4 text-destructive" />
            )}
            <span className="sr-only">Delete task</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Delete</TooltipContent>
      </Tooltip>
      <TaskDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={onDeleteOpenChange}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primary_executor_type}
        isDeleting={isDeleting}
        onConfirm={({ cascade }) => onDelete(task.id, { cascade })}
      />
      <TaskArchiveConfirmDialog
        open={showArchiveConfirm}
        onOpenChange={onArchiveOpenChange}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primary_executor_type}
        onConfirm={({ cascade }) => onArchive(task.id, { cascade })}
      />
    </div>
  );
}

function TasksPagination({
  total,
  pageCount,
  pagination,
  onPaginationChange,
}: {
  total: number;
  pageCount: number;
  pagination: PaginationState;
  onPaginationChange: (
    next: PaginationState | ((prev: PaginationState) => PaginationState),
  ) => void;
}) {
  if (total === 0) return null;
  const start = pagination.pageIndex * pagination.pageSize + 1;
  const end = Math.min((pagination.pageIndex + 1) * pagination.pageSize, total);
  const canPrevious = pagination.pageIndex > 0;
  const canNext = pagination.pageIndex + 1 < pageCount;

  return (
    <div className="flex flex-col gap-3 px-1 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
      <span>
        Showing {start} to {end} of {total} tasks
      </span>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          className="h-10 cursor-pointer sm:h-8"
          disabled={!canPrevious}
          onClick={() =>
            onPaginationChange((prev) => ({ ...prev, pageIndex: Math.max(0, prev.pageIndex - 1) }))
          }
        >
          <IconChevronLeft className="h-4 w-4" />
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-10 cursor-pointer sm:h-8"
          disabled={!canNext}
          onClick={() => onPaginationChange((prev) => ({ ...prev, pageIndex: prev.pageIndex + 1 }))}
        >
          Next
          <IconChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
