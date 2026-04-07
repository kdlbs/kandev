"use client";

import { cloneElement, isValidElement, memo, useMemo, useState } from "react";
import {
  IconChevronDown,
  IconArrowRight,
  IconPencil,
  IconCopy,
  IconArchive,
  IconTrash,
  IconLoader,
} from "@tabler/icons-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { classifyTask } from "./task-classify";
import { TaskItem } from "./task-item";

export type TaskSwitcherItem = {
  id: string;
  title: string;
  state?: TaskState;
  sessionState?: TaskSessionState;
  description?: string;
  workflowId?: string;
  workflowStepId?: string;
  repositoryPath?: string;
  repositories?: string[];
  diffStats?: { additions: number; deletions: number };
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  createdAt?: string;
  isArchived?: boolean;
  primarySessionId?: string | null;
  parentTaskTitle?: string;
  parentTaskId?: string;
  prInfo?: { number: number; state: string };
};

type StepDef = { id: string; title: string; color?: string };

type TaskSwitcherProps = {
  tasks: TaskSwitcherItem[];
  steps?: StepDef[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  deletingTaskId?: string | null;
  isLoading?: boolean;
};

type RepoGroup = {
  key: string;
  label: string;
  isMultiRepo?: boolean;
  tasks: TaskSwitcherItem[];
};

export function statePriority(task: TaskSwitcherItem): number {
  const bucket = classifyTask(task.sessionState, task.state);
  if (bucket === "review") return 0;
  if (bucket === "in_progress") return 1;
  return 2;
}

export function sortByStateThenCreated(a: TaskSwitcherItem, b: TaskSwitcherItem): number {
  // Review (turn finished) first, then in_progress, then backlog; newest createdAt within bucket
  return (
    statePriority(a) - statePriority(b) || (b.createdAt ?? "").localeCompare(a.createdAt ?? "")
  );
}

function TaskSwitcherSkeleton() {
  return (
    <div className="animate-pulse">
      <div className="h-10 bg-foreground/5" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
    </div>
  );
}

function RepoGroupHeader({
  label,
  count,
  isCollapsed,
  onToggle,
}: {
  label: string;
  count: number;
  isCollapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      data-testid={`sidebar-repo-group-${label}`}
      className="flex w-full items-center gap-2 bg-background px-3 py-1.5 cursor-pointer hover:bg-foreground/[0.03]"
    >
      <span className="flex-1 truncate text-left text-[12px] font-medium text-foreground/80">
        {label}
      </span>
      <span className="text-[11px] text-muted-foreground/50">{count}</span>
      <IconChevronDown
        className={cn(
          "h-3 w-3 text-muted-foreground/40 transition-transform",
          isCollapsed && "-rotate-90",
        )}
      />
    </button>
  );
}

function RepoGroupSection({
  group,
  subTasksByParentId,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  deletingTaskId,
}: {
  group: RepoGroup;
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  deletingTaskId?: string | null;
}) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const totalCount = group.tasks.reduce(
    (sum, t) => sum + 1 + (subTasksByParentId.get(t.id)?.length ?? 0),
    0,
  );

  function renderItem(task: TaskSwitcherItem, isSubTask?: boolean) {
    const isSelected = task.id === selectedTaskId || task.id === activeTaskId;
    const taskSteps = task.workflowId ? stepsByWorkflowId?.[task.workflowId] : undefined;
    return (
      <TaskItemWithContextMenu
        key={task.id}
        task={task}
        steps={taskSteps}
        onRenameTask={onRenameTask}
        onArchiveTask={onArchiveTask}
        onDeleteTask={onDeleteTask}
        onMoveToStep={onMoveToStep}
        isDeleting={deletingTaskId === task.id}
      >
        <TaskItem
          title={task.title}
          state={task.state}
          sessionState={task.sessionState}
          isArchived={task.isArchived}
          isSelected={isSelected}
          diffStats={task.diffStats}
          isRemoteExecutor={task.isRemoteExecutor}
          remoteExecutorType={task.remoteExecutorType}
          remoteExecutorName={task.remoteExecutorName}
          taskId={task.id}
          primarySessionId={task.primarySessionId ?? null}
          updatedAt={task.updatedAt}
          repositories={task.repositories}
          prInfo={task.prInfo}
          isSubTask={isSubTask}
          onClick={() => onSelectTask(task.id)}
          isDeleting={deletingTaskId === task.id}
        />
      </TaskItemWithContextMenu>
    );
  }

  return (
    <div>
      <RepoGroupHeader
        label={group.label}
        count={totalCount}
        isCollapsed={isCollapsed}
        onToggle={() => setIsCollapsed((v) => !v)}
      />
      {!isCollapsed &&
        group.tasks.map((task) => (
          <div key={task.id}>
            {renderItem(task)}
            {subTasksByParentId.get(task.id)?.map((sub) => renderItem(sub, true))}
          </div>
        ))}
    </div>
  );
}

function TaskItemWithContextMenu({
  task,
  steps,
  children,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  isDeleting,
}: {
  task: TaskSwitcherItem;
  steps?: StepDef[];
  children: React.ReactElement<{ menuOpen?: boolean }>;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  isDeleting?: boolean;
}) {
  const [contextOpen, setContextOpen] = useState(false);
  const [menuKey, setMenuKey] = useState(0);
  const menuOpen = contextOpen || isDeleting === true;

  return (
    <ContextMenu key={menuKey} onOpenChange={setContextOpen}>
      <ContextMenuTrigger asChild>
        <div>{cloneWithMenuOpen(children, menuOpen)}</div>
      </ContextMenuTrigger>
      <ContextMenuContent className="w-48">
        {onRenameTask && (
          <ContextMenuItem disabled={isDeleting} onClick={() => onRenameTask(task.id, task.title)}>
            <IconPencil className="mr-2 h-4 w-4" />
            Rename
          </ContextMenuItem>
        )}
        <ContextMenuItem disabled>
          <IconCopy className="mr-2 h-4 w-4" />
          Duplicate
        </ContextMenuItem>
        {onArchiveTask && (
          <ContextMenuItem disabled={isDeleting} onClick={() => onArchiveTask(task.id)}>
            <IconArchive className="mr-2 h-4 w-4" />
            Archive
          </ContextMenuItem>
        )}
        {onMoveToStep && task.workflowId && steps && steps.length > 0 && (
          <MoveToStepSubmenu
            steps={steps}
            currentStepId={task.workflowStepId}
            disabled={isDeleting}
            onSelect={(stepId) => {
              setContextOpen(false);
              setMenuKey((k) => k + 1);
              onMoveToStep(task.id, task.workflowId!, stepId);
            }}
          />
        )}
        {onDeleteTask && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem
              variant="destructive"
              disabled={isDeleting}
              onClick={() => onDeleteTask(task.id)}
            >
              {isDeleting ? (
                <IconLoader className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <IconTrash className="mr-2 h-4 w-4" />
              )}
              Delete
            </ContextMenuItem>
          </>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

function MoveToStepSubmenu({
  steps,
  currentStepId,
  disabled,
  onSelect,
}: {
  steps: StepDef[];
  currentStepId?: string;
  disabled?: boolean;
  onSelect: (stepId: string) => void;
}) {
  return (
    <>
      <ContextMenuSeparator />
      <ContextMenuSub>
        <ContextMenuSubTrigger disabled={disabled}>
          <IconArrowRight className="mr-2 h-4 w-4" />
          Move to
        </ContextMenuSubTrigger>
        <ContextMenuSubContent className="w-44">
          {steps.map((step) => (
            <ContextMenuItem
              key={step.id}
              disabled={step.id === currentStepId}
              onSelect={(e) => {
                e.preventDefault();
                onSelect(step.id);
              }}
            >
              <span className={cn("block h-2 w-2 rounded-full shrink-0", step.color)} />
              <span className="flex-1 truncate">{step.title}</span>
              {step.id === currentStepId && (
                <span className="ml-auto text-[10px] text-muted-foreground">Current</span>
              )}
            </ContextMenuItem>
          ))}
        </ContextMenuSubContent>
      </ContextMenuSub>
    </>
  );
}

function cloneWithMenuOpen(
  children: React.ReactElement<{ menuOpen?: boolean }>,
  menuOpen: boolean,
): React.ReactNode {
  if (isValidElement(children)) return cloneElement(children, { menuOpen });
  return children;
}

/** Partition root tasks into multi-repo, per-repo, and unassigned buckets. */
function partitionRootTasks(rootTasks: TaskSwitcherItem[]): {
  multiRepoTasks: TaskSwitcherItem[];
  byRepo: Map<string, TaskSwitcherItem[]>;
  unassigned: TaskSwitcherItem[];
} {
  const multiRepoTasks: TaskSwitcherItem[] = [];
  const byRepo = new Map<string, TaskSwitcherItem[]>();
  const unassigned: TaskSwitcherItem[] = [];
  for (const task of rootTasks) {
    if (task.repositories && task.repositories.length > 1) {
      multiRepoTasks.push(task);
    } else if (task.repositoryPath) {
      const arr = byRepo.get(task.repositoryPath) ?? [];
      arr.push(task);
      byRepo.set(task.repositoryPath, arr);
    } else {
      unassigned.push(task);
    }
  }
  return { multiRepoTasks, byRepo, unassigned };
}

/** Build sorted repo groups from partitioned task buckets. */
function buildRepoGroups(
  multiRepoTasks: TaskSwitcherItem[],
  byRepo: Map<string, TaskSwitcherItem[]>,
  unassigned: TaskSwitcherItem[],
): RepoGroup[] {
  const result: RepoGroup[] = [];
  if (multiRepoTasks.length > 0) {
    result.push({
      key: "multi-repo",
      label: "Multi-repo",
      isMultiRepo: true,
      tasks: [...multiRepoTasks].sort(sortByStateThenCreated),
    });
  }
  for (const [slug, groupTasks] of byRepo) {
    result.push({ key: slug, label: slug, tasks: [...groupTasks].sort(sortByStateThenCreated) });
  }
  if (unassigned.length > 0) {
    // In single-repo workspaces, merge unassigned tasks into the repo group
    // so every task appears under a meaningful label instead of "Unassigned".
    const singleRepoGroup =
      byRepo.size === 1 && multiRepoTasks.length === 0
        ? result.find((g) => g.key !== "multi-repo")
        : undefined;
    if (singleRepoGroup) {
      singleRepoGroup.tasks = [...singleRepoGroup.tasks, ...unassigned].sort(
        sortByStateThenCreated,
      );
    } else {
      result.push({
        key: "unassigned",
        label: "Unassigned",
        tasks: [...unassigned].sort(sortByStateThenCreated),
      });
    }
  }
  return result;
}

/** Group tasks by repository, separating sub-tasks from root tasks. */
function groupTasksIntoRepos(tasks: TaskSwitcherItem[]): {
  groups: RepoGroup[];
  subTasksByParentId: Map<string, TaskSwitcherItem[]>;
} {
  const allTaskIds = new Set(tasks.map((t) => t.id));
  const subMap = new Map<string, TaskSwitcherItem[]>();
  const rootTasks: TaskSwitcherItem[] = [];
  for (const t of tasks) {
    if (t.parentTaskId && allTaskIds.has(t.parentTaskId)) {
      const arr = subMap.get(t.parentTaskId) ?? [];
      arr.push(t);
      subMap.set(t.parentTaskId, arr);
    } else {
      rootTasks.push(t);
    }
  }
  const { multiRepoTasks, byRepo, unassigned } = partitionRootTasks(rootTasks);
  return {
    groups: buildRepoGroups(multiRepoTasks, byRepo, unassigned),
    subTasksByParentId: subMap,
  };
}

export const TaskSwitcher = memo(function TaskSwitcher({
  tasks,
  stepsByWorkflowId,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  deletingTaskId,
  isLoading = false,
}: TaskSwitcherProps) {
  const { groups, subTasksByParentId } = useMemo(() => groupTasksIntoRepos(tasks), [tasks]);

  if (isLoading) return <TaskSwitcherSkeleton />;
  if (tasks.length === 0) {
    return <div className="px-3 py-3 text-xs text-muted-foreground">No tasks yet.</div>;
  }

  return (
    <div>
      {groups.map((group) => (
        <RepoGroupSection
          key={group.key}
          group={group}
          subTasksByParentId={subTasksByParentId}
          stepsByWorkflowId={stepsByWorkflowId}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          onSelectTask={onSelectTask}
          onRenameTask={onRenameTask}
          onArchiveTask={onArchiveTask}
          onDeleteTask={onDeleteTask}
          onMoveToStep={onMoveToStep}
          deletingTaskId={deletingTaskId}
        />
      ))}
    </div>
  );
});
