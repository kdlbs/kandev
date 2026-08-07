"use client";

import { useMemo, type ReactNode } from "react";
import {
  IconArchive,
  IconArrowRight,
  IconLoader,
  IconLogicBuffer,
  IconTrash,
  IconUnlink,
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
import { useAppStore } from "@/components/state-provider";
import type { WorkflowStep } from "@/components/kanban-card";
import {
  stepHasAutoStart,
  type TaskMoveStep,
  type TaskMoveWorkflow,
} from "@/components/task/task-move-context-menu";
import { cn } from "@/lib/utils";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { buildEditMenuEntry } from "./kanban-card-edit-submenu";
import { buildLinkSubmenu } from "./kanban-card-link-submenu";
import { buildPrimaryPluginEntries } from "./kanban-card-plugin-menu-actions";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

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
  onLinkMergeRequest?: () => void;
  onLinkJiraTicket?: () => void;
  onLinkLinearIssue?: () => void;
  onLinkSentryIssue?: () => void;
  onMoveToStep?: (stepId: string) => void;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
  /** Defaults to an empty-id context (no visible plugin actions match it in practice). */
  pluginMenuContext?: PluginTaskMenuContext;
};

const EMPTY_PLUGIN_MENU_CONTEXT: PluginTaskMenuContext = {
  workspaceId: "",
  taskId: "",
  taskTitle: "",
  workflowStepId: null,
  presentation: "desktop",
};

function resolvePluginMenuContext(context?: PluginTaskMenuContext): PluginTaskMenuContext {
  return context ?? EMPTY_PLUGIN_MENU_CONTEXT;
}

function StepBadges({ step, isCurrent }: { step: TaskMoveStep; isCurrent: boolean }) {
  const { t } = useTranslation();
  const hasAutoStart = stepHasAutoStart(step);
  if (!isCurrent && !hasAutoStart) return null;

  return (
    <span className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
      {isCurrent && (
        <span data-testid={`task-context-step-current-${step.id}`}>{t("kanban:current")}</span>
      )}
      {hasAutoStart && (
        <span data-testid={`task-context-step-autostart-${step.id}`}>{t("kanban:autoStart")}</span>
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
    label: t("kanban:moveTo"),
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
          {t("kanban:noSteps")}
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
    label: t("kanban:sendToWorkflow"),
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
  onLinkMergeRequest,
  onLinkJiraTicket,
  onLinkLinearIssue,
  onLinkSentryIssue,
  onMoveToStep,
  onSendToWorkflow,
  pluginMenuContext,
}: BuildKanbanCardMenuEntriesArgs): KanbanCardMenuEntry[] {
  const visibleWorkflows = workflows.filter((workflow) => !workflow.hidden);
  const currentSteps = currentWorkflowId ? (stepsByWorkflowId[currentWorkflowId] ?? []) : [];
  const isProcessing = Boolean(disabled || isDeleting || isArchiving || isDetaching);
  const entries: KanbanCardMenuEntry[] = [
    buildEditMenuEntry({
      onEdit,
      disabled: isProcessing,
      context: resolvePluginMenuContext(pluginMenuContext),
    }),
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

  entries.push(
    ...buildPrimaryPluginEntries({
      disabled: isProcessing,
      context: resolvePluginMenuContext(pluginMenuContext),
    }),
  );

  const linkEntry = buildLinkSubmenu({
    disabled: isProcessing,
    onLinkPullRequest,
    onLinkIssue,
    onLinkMergeRequest,
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
    label: t("kanban:archive"),
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
    label: t("kanban:delete"),
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
    label: t("kanban:detachFromParent"),
    disabled: isProcessing,
    onSelect: onDetach,
  };
}

export function useKanbanCardMoveTargets(
  taskId: string,
  steps?: WorkflowStep[],
): KanbanCardMoveTargets {
  const workflows = useAppStore((state) => state.workflows.items);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);

  const currentWorkflowId = useMemo(() => {
    for (const [workflowId, snapshot] of Object.entries(snapshots)) {
      if (snapshot.tasks.some((task) => task.id === taskId)) return workflowId;
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
    for (const [workflowId, snapshot] of Object.entries(snapshots)) {
      result[workflowId] = snapshot.steps
        .slice()
        .sort((a, b) => a.position - b.position)
        .map((step) => ({
          id: step.id,
          title: step.title,
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
