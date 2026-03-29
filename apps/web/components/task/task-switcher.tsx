"use client";

import { memo, useMemo } from "react";
import type { ComponentType } from "react";
import {
  IconCircleCheck,
  IconCircleDashed,
  IconProgress,
  IconArrowRight,
  IconPencil,
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
  ContextMenuShortcut,
} from "@kandev/ui/context-menu";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { truncateRepoPath } from "@/lib/utils";
import { TaskItem } from "./task-item";

const SECTION_ICONS: Record<
  string,
  { Icon: ComponentType<{ className?: string }>; className: string }
> = {
  "Turn Finished": { Icon: IconCircleCheck, className: "text-green-500" },
  Running: { Icon: IconProgress, className: "text-yellow-500" },
  Backlog: { Icon: IconCircleDashed, className: "text-muted-foreground" },
};

type DiffStats = {
  additions: number;
  deletions: number;
};

type TaskSwitcherItem = {
  id: string;
  title: string;
  state?: TaskState;
  sessionState?: TaskSessionState;
  description?: string;
  workflowId?: string;
  workflowStepId?: string;
  repositoryPath?: string;
  diffStats?: DiffStats;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  createdAt?: string;
  isArchived?: boolean;
  primarySessionId?: string | null;
  parentTaskTitle?: string;
};

type TaskSwitcherProps = {
  tasks: TaskSwitcherItem[];
  steps: Array<{ id: string; title: string; color?: string }>;
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

type Section = {
  label: string;
  tasks: TaskSwitcherItem[];
};

const REVIEW_STATES = new Set<TaskSessionState>([
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);
const IN_PROGRESS_STATES = new Set<TaskSessionState>(["RUNNING"]);
const BACKLOG_STATES = new Set<TaskSessionState>(["CREATED", "STARTING"]);

const TASK_STATE_REVIEW = new Set<TaskState | undefined>(["REVIEW", "COMPLETED"]);
const TASK_STATE_IN_PROGRESS = new Set<TaskState | undefined>(["IN_PROGRESS"]);

function classifyTask(
  sessionState: TaskSessionState | undefined,
  taskState?: TaskState,
): "review" | "in_progress" | "backlog" {
  if (!sessionState) return "backlog";
  // STARTING is transient (e.g., resume after backend restart). Use the
  // task-level state so the sidebar position stays stable.
  if (sessionState === "STARTING" && taskState) {
    if (TASK_STATE_REVIEW.has(taskState)) return "review";
    if (TASK_STATE_IN_PROGRESS.has(taskState)) return "in_progress";
    return "backlog";
  }
  if (REVIEW_STATES.has(sessionState)) return "review";
  if (IN_PROGRESS_STATES.has(sessionState)) return "in_progress";
  if (BACKLOG_STATES.has(sessionState)) return "backlog";
  return "backlog";
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

function SectionHeader({ label, count }: { label: string; count: number }) {
  const icon = SECTION_ICONS[label];
  return (
    <div
      data-testid={`sidebar-section-${label}`}
      className="flex items-center justify-between px-3 py-1.5 bg-background"
    >
      <span className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground uppercase tracking-wide">
        {icon && <icon.Icon className={`h-3 w-3 ${icon.className}`} />}
        {label}
      </span>
      <span className="text-[11px] text-muted-foreground/60">{count}</span>
    </div>
  );
}

function TaskSwitcherSection({
  section,
  steps,
  stepNameById,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  onMoveToStep,
  deletingTaskId,
}: {
  section: Section;
  steps: Array<{ id: string; title: string; color?: string }>;
  stepNameById: Map<string, string>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  deletingTaskId?: string | null;
}) {
  if (section.tasks.length === 0) return null;
  return (
    <div>
      <SectionHeader label={section.label} count={section.tasks.length} />
      {section.tasks.map((task) => {
        const isActive = task.id === activeTaskId;
        const isSelected = task.id === selectedTaskId || isActive;
        const repoLabel = task.repositoryPath ? truncateRepoPath(task.repositoryPath) : undefined;
        const stepName = task.workflowStepId ? stepNameById.get(task.workflowStepId) : undefined;
        return (
          <TaskItemWithContextMenu
            key={task.id}
            task={task}
            steps={steps}
            onRenameTask={onRenameTask}
            onArchiveTask={onArchiveTask}
            onDeleteTask={onDeleteTask}
            onMoveToStep={onMoveToStep}
            isDeleting={deletingTaskId === task.id}
          >
            <TaskItem
              title={task.title}
              description={repoLabel}
              stepName={stepName}
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
              parentTaskTitle={task.parentTaskTitle}
              onClick={() => onSelectTask(task.id)}
              onRename={onRenameTask ? () => onRenameTask(task.id, task.title) : undefined}
              onArchive={onArchiveTask ? () => onArchiveTask(task.id) : undefined}
              onDelete={onDeleteTask ? () => onDeleteTask(task.id) : undefined}
              isDeleting={deletingTaskId === task.id}
            />
          </TaskItemWithContextMenu>
        );
      })}
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
  steps: Array<{ id: string; title: string; color?: string }>;
  children: React.ReactNode;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  isDeleting?: boolean;
}) {
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-48">
        {onRenameTask && (
          <ContextMenuItem onClick={() => onRenameTask(task.id, task.title)}>
            <IconPencil className="mr-2 h-4 w-4" />
            Rename
          </ContextMenuItem>
        )}
        {onArchiveTask && (
          <ContextMenuItem onClick={() => onArchiveTask(task.id)}>
            <IconArchive className="mr-2 h-4 w-4" />
            Archive
          </ContextMenuItem>
        )}
        {onMoveToStep && task.workflowId && (
          <>
            <ContextMenuSeparator />
            <ContextMenuSub>
              <ContextMenuSubTrigger>
                <IconArrowRight className="mr-2 h-4 w-4" />
                Move to
              </ContextMenuSubTrigger>
              <ContextMenuSubContent className="w-44">
                {steps.map((step) => {
                  const isCurrent = step.id === task.workflowStepId;
                  return (
                    <ContextMenuItem
                      key={step.id}
                      disabled={isCurrent}
                      onClick={() => onMoveToStep(task.id, task.workflowId!, step.id)}
                    >
                      <span
                        className="mr-2 h-2.5 w-2.5 rounded-full shrink-0"
                        style={{ backgroundColor: step.color || "var(--muted-foreground)" }}
                      />
                      {step.title}
                      {isCurrent && <ContextMenuShortcut>Current</ContextMenuShortcut>}
                    </ContextMenuItem>
                  );
                })}
              </ContextMenuSubContent>
            </ContextMenuSub>
          </>
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

export const TaskSwitcher = memo(function TaskSwitcher({
  tasks,
  steps,
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
  const stepNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const col of steps) {
      map.set(col.id, col.title);
    }
    return map;
  }, [steps]);

  const sections = useMemo(() => {
    const review: TaskSwitcherItem[] = [];
    const inProgress: TaskSwitcherItem[] = [];
    const backlog: TaskSwitcherItem[] = [];

    for (const task of tasks) {
      const bucket = classifyTask(task.sessionState, task.state);
      if (bucket === "review") review.push(task);
      else if (bucket === "in_progress") inProgress.push(task);
      else backlog.push(task);
    }

    const byCreatedAt = (a: TaskSwitcherItem, b: TaskSwitcherItem) =>
      (a.createdAt ?? "").localeCompare(b.createdAt ?? "");

    return [
      { label: "Turn Finished", tasks: review.sort(byCreatedAt) },
      { label: "Running", tasks: inProgress.sort(byCreatedAt) },
      { label: "Backlog", tasks: backlog.sort(byCreatedAt) },
    ] satisfies Section[];
  }, [tasks]);

  if (isLoading) {
    return <TaskSwitcherSkeleton />;
  }

  if (tasks.length === 0) {
    return <div className="px-3 py-3 text-xs text-muted-foreground">No tasks yet.</div>;
  }

  return (
    <div>
      {sections.map((section) => (
        <TaskSwitcherSection
          key={section.label}
          section={section}
          steps={steps}
          stepNameById={stepNameById}
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
