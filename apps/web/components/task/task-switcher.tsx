"use client";

import { memo, useMemo } from "react";
import type { ComponentType } from "react";
import { IconCircleCheck, IconCircleDashed, IconProgress } from "@tabler/icons-react";
import type { TaskState, TaskSessionState } from "@/lib/types/http";
import { truncateRepoPath } from "@/lib/utils";
import { TaskItem } from "./task-item";

const SECTION_ICONS: Record<string, { Icon: ComponentType<{ className?: string }>; className: string }> = {
  Review: { Icon: IconCircleCheck, className: "text-green-500" },
  "In Progress": { Icon: IconProgress, className: "text-yellow-500" },
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
  workflowStepId?: string;
  repositoryPath?: string;
  diffStats?: DiffStats;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string;
  remoteExecutorName?: string;
  updatedAt?: string;
  isArchived?: boolean;
  primarySessionId?: string | null;
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

function classifyTask(
  sessionState: TaskSessionState | undefined,
): "review" | "in_progress" | "backlog" {
  if (!sessionState) return "backlog";
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
    <div className="flex items-center justify-between px-3 py-1.5 bg-foreground/[0.03]">
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
  stepNameById,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  onRenameTask,
  onArchiveTask,
  onDeleteTask,
  deletingTaskId,
}: {
  section: Section;
  stepNameById: Map<string, string>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onDeleteTask?: (taskId: string) => void;
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
          <TaskItem
            key={task.id}
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
            onClick={() => onSelectTask(task.id)}
            onRename={onRenameTask ? () => onRenameTask(task.id, task.title) : undefined}
            onArchive={onArchiveTask ? () => onArchiveTask(task.id) : undefined}
            onDelete={onDeleteTask ? () => onDeleteTask(task.id) : undefined}
            isDeleting={deletingTaskId === task.id}
          />
        );
      })}
    </div>
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
      const bucket = classifyTask(task.sessionState);
      if (bucket === "review") review.push(task);
      else if (bucket === "in_progress") inProgress.push(task);
      else backlog.push(task);
    }

    const byTitle = (a: TaskSwitcherItem, b: TaskSwitcherItem) => a.title.localeCompare(b.title);
    review.sort(byTitle);
    inProgress.sort(byTitle);
    backlog.sort(byTitle);

    return [
      { label: "Review", tasks: review },
      { label: "In Progress", tasks: inProgress },
      { label: "Backlog", tasks: backlog },
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
          stepNameById={stepNameById}
          activeTaskId={activeTaskId}
          selectedTaskId={selectedTaskId}
          onSelectTask={onSelectTask}
          onRenameTask={onRenameTask}
          onArchiveTask={onArchiveTask}
          onDeleteTask={onDeleteTask}
          deletingTaskId={deletingTaskId}
        />
      ))}
    </div>
  );
});
