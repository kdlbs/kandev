"use client";

import { useMemo, useState } from "react";
import type { PaginationState } from "@tanstack/react-table";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconArchive,
  IconChevronLeft,
  IconChevronRight,
  IconLoader,
  IconTrash,
} from "@tabler/icons-react";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { primaryTaskRepository, type Repository, type Task, type Workflow } from "@/lib/types/http";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import { formatRelativeTime } from "@/lib/utils";
import {
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
  TASK_STATE_ORDER,
  type TasksListGroup,
  type TasksListSort,
} from "@/lib/tasks/tasks-list-options";

export type TasksListViewProps = {
  total: number;
  showArchived: boolean;
  setShowArchived: (show: boolean) => void;
  tasksListSort: TasksListSort;
  onTasksListSortChange: (sort: TasksListSort) => void;
  tasksListGroup: TasksListGroup;
  onTasksListGroupChange: (group: TasksListGroup) => void;
  tasks: Task[];
  workflows: Workflow[];
  repositories: Repository[];
  pageCount: number;
  pagination: PaginationState;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  isLoading: boolean;
  handleRowClick: (task: Task) => void;
  deletingTaskId: string | null;
  handleArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  handleDelete: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
};

export function TasksListView({
  total,
  showArchived,
  setShowArchived,
  tasksListSort,
  onTasksListSortChange,
  tasksListGroup,
  onTasksListGroupChange,
  tasks,
  workflows,
  repositories,
  pageCount,
  pagination,
  setPagination,
  isLoading,
  handleRowClick,
  deletingTaskId,
  handleArchive,
  handleDelete,
}: TasksListViewProps) {
  return (
    <main className="flex-1 overflow-auto px-4 py-4 sm:px-6 sm:py-6">
      <div className="space-y-4">
        <TasksListControls
          showArchived={showArchived}
          onShowArchivedChange={setShowArchived}
          tasksListSort={tasksListSort}
          onTasksListSortChange={onTasksListSortChange}
          tasksListGroup={tasksListGroup}
          onTasksListGroupChange={onTasksListGroupChange}
        />
        <TaskRows
          tasks={tasks}
          workflows={workflows}
          repositories={repositories}
          tasksListGroup={tasksListGroup}
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

function TasksListControls({
  showArchived,
  onShowArchivedChange,
  tasksListSort,
  onTasksListSortChange,
  tasksListGroup,
  onTasksListGroupChange,
}: {
  showArchived: boolean;
  onShowArchivedChange: (show: boolean) => void;
  tasksListSort: TasksListSort;
  onTasksListSortChange: (sort: TasksListSort) => void;
  tasksListGroup: TasksListGroup;
  onTasksListGroupChange: (group: TasksListGroup) => void;
}) {
  return (
    <div className="flex min-h-9 flex-wrap items-center justify-end gap-3">
      <ListOptionSelect
        label="Sort"
        value={tasksListSort}
        options={TASKS_LIST_SORT_OPTIONS}
        onChange={(value) => onTasksListSortChange(value as TasksListSort)}
        testId="tasks-list-sort"
      />
      <ListOptionSelect
        label="Group"
        value={tasksListGroup}
        options={TASKS_LIST_GROUP_OPTIONS}
        onChange={(value) => onTasksListGroupChange(value as TasksListGroup)}
        testId="tasks-list-group"
      />
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

function ListOptionSelect<T extends string>({
  label,
  value,
  options,
  onChange,
  testId,
}: {
  label: string;
  value: T;
  options: ReadonlyArray<{ readonly value: T; readonly label: string }>;
  onChange: (value: T) => void;
  testId: string;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-muted-foreground">{label}</span>
      <Select value={value} onValueChange={(next) => onChange(next as T)}>
        <SelectTrigger data-testid={testId} className="h-10 w-[150px] cursor-pointer lg:h-9">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value} className="cursor-pointer">
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

type TaskTreeNode = {
  task: Task;
  children: TaskTreeNode[];
  level: number;
};

type TaskListSection = {
  key: string;
  title: string | null;
  nodes: TaskTreeNode[];
};

function TaskRows({
  tasks,
  workflows,
  repositories,
  tasksListGroup,
  isLoading,
  deletingTaskId,
  onArchive,
  onDelete,
  onRowClick,
}: {
  tasks: Task[];
  workflows: Workflow[];
  repositories: Repository[];
  tasksListGroup: TasksListGroup;
  isLoading: boolean;
  deletingTaskId: string | null;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onDelete: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onRowClick: (task: Task) => void;
}) {
  const workflowMap = useMemo(() => new Map(workflows.map((w) => [w.id, w.name])), [workflows]);
  const repoMap = useMemo(() => new Map(repositories.map((r) => [r.id, r.name])), [repositories]);
  const sections = useMemo(
    () => buildTaskSections(tasks, { groupBy: tasksListGroup, workflowMap, repoMap }),
    [repoMap, tasks, tasksListGroup, workflowMap],
  );

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
    <div className="space-y-5" data-testid="tasks-list">
      {sections.map((section) => (
        <TaskListSectionView
          key={section.key}
          section={section}
          deletingTaskId={deletingTaskId}
          onArchive={onArchive}
          onDelete={onDelete}
          onRowClick={onRowClick}
        />
      ))}
    </div>
  );
}

function buildTaskSections(
  tasks: Task[],
  {
    groupBy,
    workflowMap,
    repoMap,
  }: {
    groupBy: TasksListGroup;
    workflowMap: Map<string, string>;
    repoMap: Map<string, string>;
  },
): TaskListSection[] {
  const roots = buildTaskTree(tasks);
  if (groupBy === "none") {
    return [{ key: "all", title: null, nodes: roots }];
  }

  const sections = new Map<string, TaskListSection>();
  for (const node of roots) {
    const { key, title } = groupForTask(node.task, groupBy, workflowMap, repoMap);
    const section = sections.get(key) ?? { key, title, nodes: [] };
    section.nodes.push(node);
    sections.set(key, section);
  }

  return Array.from(sections.values()).sort((a, b) => compareSection(a, b, groupBy));
}

function buildTaskTree(tasks: Task[]): TaskTreeNode[] {
  const childrenByParent = new Map<string, Task[]>();
  const taskIds = new Set(tasks.map((task) => task.id));
  const roots: Task[] = [];

  for (const task of tasks) {
    if (task.parent_id && taskIds.has(task.parent_id)) {
      const siblings = childrenByParent.get(task.parent_id) ?? [];
      siblings.push(task);
      childrenByParent.set(task.parent_id, siblings);
    } else {
      roots.push(task);
    }
  }

  const visited = new Set<string>();

  const buildNode = (task: Task, level: number): TaskTreeNode | null => {
    if (visited.has(task.id)) return null;
    visited.add(task.id);
    return {
      task,
      level,
      children: (childrenByParent.get(task.id) ?? [])
        .map((child) => buildNode(child, level + 1))
        .filter((node): node is TaskTreeNode => node !== null),
    };
  };

  const nodes = roots
    .map((task) => buildNode(task, 0))
    .filter((node): node is TaskTreeNode => node !== null);
  for (const task of tasks) {
    const node = buildNode(task, 0);
    if (node) nodes.push(node);
  }

  return nodes;
}

function groupForTask(
  task: Task,
  groupBy: TasksListGroup,
  workflowMap: Map<string, string>,
  repoMap: Map<string, string>,
) {
  if (groupBy === "workflow") {
    const title = workflowMap.get(task.workflow_id) ?? "No workflow";
    return { key: `workflow:${task.workflow_id || "none"}`, title };
  }
  if (groupBy === "repository") {
    const primaryRepo = primaryTaskRepository(task.repositories);
    const repoId = primaryRepo?.repository_id ?? "none";
    const title = primaryRepo ? (repoMap.get(repoId) ?? "No repository") : "No repository";
    return { key: `repository:${repoId}`, title };
  }
  const title = formatTaskStateLabel(task.state);
  return { key: `state:${task.state}`, title };
}

function compareSection(a: TaskListSection, b: TaskListSection, groupBy: TasksListGroup): number {
  if (groupBy === "state") {
    const aIndex = TASK_STATE_ORDER.indexOf(a.key.replace("state:", "") as Task["state"]);
    const bIndex = TASK_STATE_ORDER.indexOf(b.key.replace("state:", "") as Task["state"]);
    return (
      (aIndex === -1 ? Number.MAX_SAFE_INTEGER : aIndex) -
      (bIndex === -1 ? Number.MAX_SAFE_INTEGER : bIndex)
    );
  }
  return (a.title ?? "").localeCompare(b.title ?? "", undefined, { sensitivity: "base" });
}

function flattenTaskTree(nodes: TaskTreeNode[]): TaskTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenTaskTree(node.children)]);
}

function TaskListRow({
  task,
  level,
  deletingTaskId,
  onArchive,
  onDelete,
  onRowClick,
}: {
  task: Task;
  level: number;
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
      data-level={level}
      className="grid min-h-12 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-4 py-2 text-sm transition-colors hover:bg-muted/60 cursor-pointer"
      onClick={() => onRowClick(task)}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return;
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onRowClick(task);
        }
      }}
    >
      <div
        className="flex min-w-0 items-center gap-2"
        data-testid="tasks-list-row-content"
        style={{ paddingLeft: `${level * 28}px` }}
      >
        {getTaskStateIcon(task.state, "h-4 w-4 shrink-0")}
        <span className="min-w-0 truncate font-medium" data-testid="tasks-list-row-title">
          {task.title}
        </span>
        {isArchived && (
          <Badge
            variant="outline"
            className="shrink-0 border-amber-500/30 px-1.5 py-0 text-[10px] text-amber-500"
          >
            Archived
          </Badge>
        )}
      </div>
      <div className="flex items-center justify-between gap-3 md:justify-end">
        <span className="hidden text-xs text-muted-foreground sm:inline">
          {formatRelativeTime(task.updated_at)}
        </span>
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

function TaskListSectionView({
  section,
  deletingTaskId,
  onArchive,
  onDelete,
  onRowClick,
}: {
  section: TaskListSection;
  deletingTaskId: string | null;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onDelete: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onRowClick: (task: Task) => void;
}) {
  const rows = flattenTaskTree(section.nodes);
  return (
    <section className="space-y-2" data-testid="tasks-list-section">
      {section.title && (
        <div className="flex items-center gap-2 px-1 text-xs font-semibold uppercase tracking-normal text-muted-foreground">
          <span>{section.title}</span>
          <span className="text-muted-foreground/70">{rows.length}</span>
        </div>
      )}
      <div className="rounded-lg border border-border divide-y divide-border overflow-hidden">
        {rows.map(({ task, level }) => (
          <TaskListRow
            key={task.id}
            task={task}
            level={level}
            deletingTaskId={deletingTaskId}
            onArchive={onArchive}
            onDelete={onDelete}
            onRowClick={onRowClick}
          />
        ))}
      </div>
    </section>
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
          <span tabIndex={isDeleting ? 0 : -1} className="inline-flex">
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
          </span>
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
  if (total === 0 || pageCount <= 1) return null;
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
