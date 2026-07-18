"use client";

import { useCallback, useMemo, useRef, useSyncExternalStore, type ReactNode } from "react";
import { useQueryClient, type QueryClient, type QueryKey } from "@tanstack/react-query";
import {
  IconArchive,
  IconArrowRight,
  IconBrandSentry,
  IconCircleDot,
  IconGitPullRequest,
  IconLink,
  IconLoader,
  IconLogicBuffer,
  IconPencil,
  IconTicket,
  IconTrash,
  IconUnlink,
} from "@tabler/icons-react";
import { useAllCachedWorkflows } from "@/hooks/use-workflow-cache";
import type { WorkflowStep } from "@/components/kanban-card";
import {
  stepHasAutoStart,
  type TaskMoveStep,
  type TaskMoveWorkflow,
} from "@/components/task/task-move-context-menu";
import { cn } from "@/lib/utils";
import type { WorkflowSnapshot } from "@/lib/types/http";

type ItemEntry = {
  kind: "item";
  key: string;
  label: ReactNode;
  icon?: ReactNode;
  leading?: ReactNode;
  trailing?: ReactNode;
  disabled?: boolean;
  destructive?: boolean;
  testId?: string;
  onSelect?: () => void;
};

type SeparatorEntry = { kind: "separator"; key: string };

type SubmenuEntry = {
  kind: "submenu";
  key: string;
  label: ReactNode;
  icon?: ReactNode;
  disabled?: boolean;
  testId?: string;
  className?: string;
  children: KanbanCardMenuEntry[];
};

export type KanbanCardMenuEntry = ItemEntry | SeparatorEntry | SubmenuEntry;

export type KanbanCardMoveTargets = {
  currentWorkflowId: string | null;
  workflowItems: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
};

type WorkflowSnapshotCache = {
  signature: string;
  snapshots: WorkflowSnapshot[];
};

const EMPTY_SNAPSHOTS: WorkflowSnapshot[] = [];

type BuildKanbanCardMenuEntriesArgs = {
  currentWorkflowId?: string | null;
  currentStepId?: string | null;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isDetaching?: boolean;
  parentTaskId?: string | null;
  onEdit?: () => void;
  onArchive?: () => void;
  onDelete?: () => void;
  onDetach?: () => void;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
  onLinkJiraTicket?: () => void;
  onLinkLinearIssue?: () => void;
  onLinkSentryIssue?: () => void;
  onMoveToStep?: (stepId: string) => void;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
};

function StepBadges({ step, isCurrent }: { step: TaskMoveStep; isCurrent: boolean }) {
  const hasAutoStart = stepHasAutoStart(step);
  if (!isCurrent && !hasAutoStart) return null;

  return (
    <span className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
      {isCurrent && <span data-testid={`task-context-step-current-${step.id}`}>Current</span>}
      {hasAutoStart && (
        <span data-testid={`task-context-step-autostart-${step.id}`}>Auto-start</span>
      )}
    </span>
  );
}

function buildStepEntry(
  step: TaskMoveStep,
  currentStepId: string | null | undefined,
  onSelect: (stepId: string) => void,
): KanbanCardMenuEntry {
  const isCurrent = step.id === currentStepId;
  return {
    kind: "item",
    key: `step-${step.id}`,
    testId: `task-context-step-${step.id}`,
    disabled: isCurrent,
    leading: <span className={cn("block h-2 w-2 rounded-full shrink-0", step.color ?? "")} />,
    label: <span className="flex-1 truncate">{step.title}</span>,
    trailing: <StepBadges step={step} isCurrent={isCurrent} />,
    onSelect: () => {
      if (!isCurrent) onSelect(step.id);
    },
  };
}

function buildMoveToCurrentWorkflowSubmenu({
  steps,
  currentStepId,
  disabled,
  onMoveToStep,
}: {
  steps: TaskMoveStep[];
  currentStepId?: string | null;
  disabled?: boolean;
  onMoveToStep?: (stepId: string) => void;
}): KanbanCardMenuEntry | null {
  if (!onMoveToStep || steps.length <= 1) return null;
  return {
    kind: "submenu",
    key: "move-to",
    testId: "task-context-move-to",
    icon: <IconArrowRight className="mr-2 h-4 w-4" />,
    label: "Move to",
    disabled,
    className: "w-48",
    children: steps.map((step) => buildStepEntry(step, currentStepId, onMoveToStep)),
  };
}

function buildWorkflowTargetEntry({
  workflow,
  steps,
  disabled,
  onSendToWorkflow,
}: {
  workflow: TaskMoveWorkflow;
  steps: TaskMoveStep[];
  disabled?: boolean;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
}): KanbanCardMenuEntry {
  if (steps.length === 0 || !onSendToWorkflow) {
    return {
      kind: "item",
      key: `workflow-${workflow.id}`,
      testId: `task-context-workflow-${workflow.id}`,
      disabled: true,
      label: <span className="flex-1 truncate">{workflow.name}</span>,
      trailing: (
        <span data-testid="task-context-disabled-reason" className="ml-2 text-[10px]">
          No steps
        </span>
      ),
    };
  }

  return {
    kind: "submenu",
    key: `workflow-${workflow.id}`,
    testId: `task-context-workflow-${workflow.id}`,
    label: <span className="truncate">{workflow.name}</span>,
    disabled,
    className: "w-48",
    children: steps.map((step) =>
      buildStepEntry(step, null, (stepId) => onSendToWorkflow(workflow.id, stepId)),
    ),
  };
}

function buildSendToWorkflowSubmenu({
  currentWorkflowId,
  workflows,
  stepsByWorkflowId,
  disabled,
  onSendToWorkflow,
}: {
  currentWorkflowId?: string | null;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  disabled?: boolean;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
}): KanbanCardMenuEntry | null {
  const targets = workflows.filter((workflow) => workflow.id !== currentWorkflowId);
  if (!onSendToWorkflow || !currentWorkflowId || targets.length === 0) return null;
  return {
    kind: "submenu",
    key: "send-to-workflow",
    testId: "task-context-send-to-workflow",
    icon: <IconLogicBuffer className="mr-2 h-4 w-4" />,
    label: "Send to workflow",
    disabled,
    className: "w-56",
    children: targets.map((workflow) =>
      buildWorkflowTargetEntry({
        workflow,
        steps: stepsByWorkflowId[workflow.id] ?? [],
        disabled,
        onSendToWorkflow,
      }),
    ),
  };
}

function buildLinkSubmenu({
  disabled,
  onLinkPullRequest,
  onLinkIssue,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
}: {
  disabled?: boolean;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
  onLinkJiraTicket?: () => void;
  onLinkLinearIssue?: () => void;
  onLinkSentryIssue?: () => void;
}): KanbanCardMenuEntry | null {
  if (
    !onLinkPullRequest &&
    !onLinkIssue &&
    !onLinkJiraTicket &&
    !onLinkLinearIssue &&
    !onLinkSentryIssue
  ) {
    return null;
  }
  const children: KanbanCardMenuEntry[] = [];
  if (onLinkPullRequest) {
    children.push({
      kind: "item",
      key: "link-github-pull-request",
      testId: "task-context-link-github-pull-request",
      icon: <IconGitPullRequest className="mr-2 h-4 w-4" />,
      label: "GitHub Pull Request",
      disabled,
      onSelect: onLinkPullRequest,
    });
  }
  if (onLinkIssue) {
    children.push({
      kind: "item",
      key: "link-github-issue",
      testId: "task-context-link-github-issue",
      icon: <IconCircleDot className="mr-2 h-4 w-4" />,
      label: "GitHub Issue",
      disabled,
      onSelect: onLinkIssue,
    });
  }
  if (onLinkJiraTicket) {
    children.push({
      kind: "item",
      key: "link-jira-ticket",
      testId: "task-context-link-jira-ticket",
      icon: <IconTicket className="mr-2 h-4 w-4" />,
      label: "Jira Ticket",
      disabled,
      onSelect: onLinkJiraTicket,
    });
  }
  if (onLinkLinearIssue) {
    children.push({
      kind: "item",
      key: "link-linear-issue",
      testId: "task-context-link-linear-issue",
      icon: <IconCircleDot className="mr-2 h-4 w-4" />,
      label: "Linear Issue",
      disabled,
      onSelect: onLinkLinearIssue,
    });
  }
  if (onLinkSentryIssue) {
    children.push({
      kind: "item",
      key: "link-sentry-issue",
      testId: "task-context-link-sentry-issue",
      icon: <IconBrandSentry className="mr-2 h-4 w-4" />,
      label: "Sentry Issue",
      disabled,
      onSelect: onLinkSentryIssue,
    });
  }
  return {
    kind: "submenu",
    key: "link",
    testId: "task-context-link",
    icon: <IconLink className="mr-2 h-4 w-4" />,
    label: "Link",
    disabled,
    className: "w-56",
    children,
  };
}

export function buildKanbanCardMenuEntries({
  currentWorkflowId,
  currentStepId,
  workflows,
  stepsByWorkflowId,
  disabled,
  isDeleting,
  isArchiving,
  isDetaching,
  parentTaskId,
  onEdit,
  onArchive,
  onDelete,
  onDetach,
  onLinkPullRequest,
  onLinkIssue,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  onMoveToStep,
  onSendToWorkflow,
}: BuildKanbanCardMenuEntriesArgs): KanbanCardMenuEntry[] {
  const visibleWorkflows = workflows.filter((workflow) => !workflow.hidden);
  const currentSteps = currentWorkflowId ? (stepsByWorkflowId[currentWorkflowId] ?? []) : [];
  const isProcessing = Boolean(disabled || isDeleting || isArchiving || isDetaching);
  const entries: KanbanCardMenuEntry[] = [
    {
      kind: "item",
      key: "edit",
      icon: <IconPencil className="mr-2 h-4 w-4" />,
      label: "Edit",
      disabled: isProcessing || !onEdit,
      onSelect: onEdit,
    },
  ];

  const moveToEntry = buildMoveToCurrentWorkflowSubmenu({
    steps: currentSteps,
    currentStepId,
    disabled: isProcessing,
    onMoveToStep,
  });
  if (moveToEntry) entries.push(moveToEntry);

  const sendToEntry = buildSendToWorkflowSubmenu({
    currentWorkflowId,
    workflows: visibleWorkflows,
    stepsByWorkflowId,
    disabled: isProcessing,
    onSendToWorkflow,
  });
  if (sendToEntry) entries.push(sendToEntry);

  const linkEntry = buildLinkSubmenu({
    disabled: isProcessing,
    onLinkPullRequest,
    onLinkIssue,
    onLinkJiraTicket,
    onLinkLinearIssue,
    onLinkSentryIssue,
  });
  if (linkEntry) entries.push(linkEntry);

  entries.push({
    kind: "item",
    key: "archive",
    icon: isArchiving ? (
      <IconLoader className="mr-2 h-4 w-4 animate-spin" />
    ) : (
      <IconArchive className="mr-2 h-4 w-4" />
    ),
    label: "Archive",
    disabled: isProcessing || !onArchive,
    onSelect: onArchive,
  });

  const detachEntry = buildDetachEntry({ parentTaskId, onDetach, isDetaching, isProcessing });
  if (detachEntry) entries.push(detachEntry);

  entries.push({ kind: "separator", key: "delete-separator" });
  entries.push({
    kind: "item",
    key: "delete",
    icon: isDeleting ? (
      <IconLoader className="mr-2 h-4 w-4 animate-spin" />
    ) : (
      <IconTrash className="mr-2 h-4 w-4" />
    ),
    label: "Delete",
    destructive: true,
    disabled: isProcessing || !onDelete,
    onSelect: onDelete,
  });

  return entries;
}

function buildDetachEntry({
  parentTaskId,
  onDetach,
  isDetaching,
  isProcessing,
}: Pick<BuildKanbanCardMenuEntriesArgs, "parentTaskId" | "onDetach" | "isDetaching"> & {
  isProcessing: boolean;
}): KanbanCardMenuEntry | null {
  if (!parentTaskId || !onDetach) return null;
  return {
    kind: "item",
    key: "detach",
    testId: "task-context-detach",
    icon: isDetaching ? (
      <IconLoader className="mr-2 h-4 w-4 animate-spin" />
    ) : (
      <IconUnlink className="mr-2 h-4 w-4" />
    ),
    label: "Detach from parent",
    disabled: isProcessing,
    onSelect: onDetach,
  };
}

function isWorkflowSnapshotQueryKey(key: QueryKey): boolean {
  return Array.isArray(key) && key[0] === "workflows" && key[2] === "snapshot";
}

function isWorkflowSnapshot(value: unknown): value is WorkflowSnapshot {
  return (
    typeof value === "object" &&
    value !== null &&
    "workflow" in value &&
    "steps" in value &&
    "tasks" in value &&
    Array.isArray((value as { tasks?: unknown }).tasks)
  );
}

function readWorkflowSnapshots(client: QueryClient): WorkflowSnapshotCache {
  const queries = client
    .getQueryCache()
    .findAll()
    .filter((query) => isWorkflowSnapshotQueryKey(query.queryKey))
    .sort((a, b) => a.queryHash.localeCompare(b.queryHash));
  const snapshots = queries
    .map((query) => query.state.data)
    .filter((data): data is WorkflowSnapshot => isWorkflowSnapshot(data));

  return {
    signature: queries
      .map(
        (query) => `${query.queryHash}:${query.state.dataUpdatedAt}:${query.state.dataUpdateCount}`,
      )
      .join("|"),
    snapshots,
  };
}

function useCachedWorkflowSnapshots(): WorkflowSnapshot[] {
  const queryClient = useQueryClient();
  const snapshotRef = useRef<WorkflowSnapshotCache>({
    signature: "",
    snapshots: EMPTY_SNAPSHOTS,
  });
  const getSnapshot = useCallback(() => {
    const snapshot = readWorkflowSnapshots(queryClient);
    if (snapshot.signature === snapshotRef.current.signature) {
      return snapshotRef.current.snapshots;
    }
    snapshotRef.current = snapshot;
    return snapshot.snapshots;
  }, [queryClient]);

  return useSyncExternalStore(
    (onStoreChange) => queryClient.getQueryCache().subscribe(onStoreChange),
    getSnapshot,
    () => EMPTY_SNAPSHOTS,
  );
}

export function useKanbanCardMoveTargets(
  taskId: string,
  steps?: WorkflowStep[],
): KanbanCardMoveTargets {
  const workflows = useAllCachedWorkflows();
  const snapshots = useCachedWorkflowSnapshots();

  const currentWorkflowId = useMemo(() => {
    for (const snapshot of snapshots) {
      if (snapshot.tasks.some((task) => task.id === taskId)) return snapshot.workflow.id;
    }
    return null;
  }, [snapshots, taskId]);

  const workflowItems = useMemo<TaskMoveWorkflow[]>(() => {
    const current = workflows.find((workflow) => workflow.id === currentWorkflowId);
    return workflows
      .filter((workflow) => workflow.workspaceId === current?.workspaceId && !workflow.hidden)
      .map((workflow) => ({ id: workflow.id, name: workflow.name, hidden: workflow.hidden }));
  }, [workflows, currentWorkflowId]);

  const stepsByWorkflowId = useMemo<Record<string, TaskMoveStep[]>>(() => {
    const result: Record<string, TaskMoveStep[]> = {};
    for (const snapshot of snapshots) {
      result[snapshot.workflow.id] = snapshot.steps
        .slice()
        .sort((a, b) => a.position - b.position)
        .map((step) => ({
          id: step.id,
          title: step.name,
          color: step.color,
          events: step.events,
        }));
    }
    if (currentWorkflowId && steps) {
      result[currentWorkflowId] = steps.map((step) => ({
        id: step.id,
        title: step.title,
        color: step.color,
        events: step.events,
      }));
    }
    return result;
  }, [snapshots, currentWorkflowId, steps]);

  return { currentWorkflowId, workflowItems, stepsByWorkflowId };
}

export {
  KanbanCardContextMenuItems,
  KanbanCardDropdownMenuItems,
} from "./kanban-card-menu-renderers";
