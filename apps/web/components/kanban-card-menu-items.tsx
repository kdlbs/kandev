"use client";

import { useCallback, useMemo, useRef, useSyncExternalStore, type ReactNode } from "react";
import { useQueryClient, type QueryClient, type QueryKey } from "@tanstack/react-query";
import {
  IconArchive,
  IconArrowRight,
  IconCircleDot,
  IconGitPullRequest,
  IconLink,
  IconLoader,
  IconLogicBuffer,
  IconPencil,
  IconTrash,
} from "@tabler/icons-react";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import {
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
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
  onEdit?: () => void;
  onArchive?: () => void;
  onDelete?: () => void;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
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
}: {
  disabled?: boolean;
  onLinkPullRequest?: () => void;
  onLinkIssue?: () => void;
}): KanbanCardMenuEntry | null {
  if (!onLinkPullRequest && !onLinkIssue) return null;
  return {
    kind: "submenu",
    key: "link",
    testId: "task-context-link",
    icon: <IconLink className="mr-2 h-4 w-4" />,
    label: "Link",
    disabled,
    className: "w-56",
    children: [
      {
        kind: "item",
        key: "link-github-pull-request",
        testId: "task-context-link-github-pull-request",
        icon: <IconGitPullRequest className="mr-2 h-4 w-4" />,
        label: "GitHub Pull Request",
        disabled: disabled || !onLinkPullRequest,
        onSelect: onLinkPullRequest,
      },
      {
        kind: "item",
        key: "link-github-issue",
        testId: "task-context-link-github-issue",
        icon: <IconCircleDot className="mr-2 h-4 w-4" />,
        label: "GitHub Issue",
        disabled: disabled || !onLinkIssue,
        onSelect: onLinkIssue,
      },
    ],
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
  onEdit,
  onArchive,
  onDelete,
  onLinkPullRequest,
  onLinkIssue,
  onMoveToStep,
  onSendToWorkflow,
}: BuildKanbanCardMenuEntriesArgs): KanbanCardMenuEntry[] {
  const visibleWorkflows = workflows.filter((workflow) => !workflow.hidden);
  const currentSteps = currentWorkflowId ? (stepsByWorkflowId[currentWorkflowId] ?? []) : [];
  const isProcessing = Boolean(disabled || isDeleting || isArchiving);
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

function ContextEntry({ entry }: { entry: KanbanCardMenuEntry }) {
  if (entry.kind === "separator") return <ContextMenuSeparator />;
  if (entry.kind === "submenu") {
    return (
      <ContextMenuSub>
        <ContextMenuSubTrigger data-testid={entry.testId} disabled={entry.disabled}>
          {entry.icon}
          {entry.label}
        </ContextMenuSubTrigger>
        <ContextMenuSubContent className={entry.className}>
          {entry.children.map((child) => (
            <ContextEntry key={child.key} entry={child} />
          ))}
        </ContextMenuSubContent>
      </ContextMenuSub>
    );
  }

  return (
    <ContextMenuItem
      data-testid={entry.testId}
      disabled={entry.disabled}
      className={entry.destructive ? "text-destructive focus:text-destructive" : undefined}
      // React events bubble through the React tree even from a portal — stop here so the card's onClick doesn't navigate.
      onClick={(event) => event.stopPropagation()}
      onSelect={() => {
        if (!entry.disabled) entry.onSelect?.();
      }}
    >
      {entry.icon}
      {entry.leading}
      {entry.label}
      {entry.trailing}
    </ContextMenuItem>
  );
}

function DropdownEntry({ entry }: { entry: KanbanCardMenuEntry }) {
  if (entry.kind === "separator") return <DropdownMenuSeparator />;
  if (entry.kind === "submenu") {
    return (
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          data-testid={entry.testId}
          disabled={entry.disabled}
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          {entry.icon}
          {entry.label}
        </DropdownMenuSubTrigger>
        <DropdownMenuPortal>
          <DropdownMenuSubContent className={entry.className}>
            {entry.children.map((child) => (
              <DropdownEntry key={child.key} entry={child} />
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuPortal>
      </DropdownMenuSub>
    );
  }

  return (
    <DropdownMenuItem
      data-testid={entry.testId}
      disabled={entry.disabled}
      className={entry.destructive ? "text-destructive focus:text-destructive" : undefined}
      // React events bubble through the React tree even from a portal - stop here so click/pointer don't reach the parent Card's onClick or dnd-kit listeners.
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onSelect={(event) => {
        event.stopPropagation();
        if (!entry.disabled) entry.onSelect?.();
      }}
    >
      {entry.icon}
      {entry.leading}
      {entry.label}
      {entry.trailing}
    </DropdownMenuItem>
  );
}

export function KanbanCardContextMenuItems({ entries }: { entries: KanbanCardMenuEntry[] }) {
  return (
    <>
      {entries.map((entry) => (
        <ContextEntry key={entry.key} entry={entry} />
      ))}
    </>
  );
}

export function KanbanCardDropdownMenuItems({ entries }: { entries: KanbanCardMenuEntry[] }) {
  return (
    <>
      {entries.map((entry) => (
        <DropdownEntry key={entry.key} entry={entry} />
      ))}
    </>
  );
}
