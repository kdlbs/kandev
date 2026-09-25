"use client";

import { type ReactNode } from "react";
import {
  IconArchive,
  IconArrowRight,
  IconFlag,
  IconLoader,
  IconLogicBuffer,
  IconTrash,
  IconUnlink,
} from "@tabler/icons-react";
import {
  stepHasAutoStart,
  type TaskMoveStep,
  type TaskMoveWorkflow,
} from "@/components/task/task-move-context-menu";
import {
  isTaskPriority,
  TASK_PRIORITY_LABEL_KEYS,
  TASK_PRIORITY_TOKENS,
} from "@/lib/tasks/task-priority";
import type { TaskPriority } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { buildLinkSubmenu } from "./kanban-card-link-submenu";
import type { PluginIcon, PluginTaskMenuContext } from "@/lib/plugins/types";
import { buildEditMenuEntry } from "./kanban-card-edit-submenu";
import { buildPrimaryPluginEntries } from "./plugins/task-menu-actions";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { buildSingleTaskWorkflowEntry } from "./kanban-card-single-workflow-entry";
export {
  KanbanCardContextMenuItems,
  KanbanCardDropdownMenuItems,
} from "./kanban-card-menu-entry-renderers";
export { useKanbanCardMoveTargets } from "@/hooks/use-kanban-card-move-targets";

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

export type KanbanCardMenuEntryGroup = {
  key: string;
  entries: readonly KanbanCardMenuEntry[];
};

/** Inserts one separator between each nonempty top-level menu group. */
export function buildGroupedMenuEntries(
  groups: readonly KanbanCardMenuEntryGroup[],
): KanbanCardMenuEntry[] {
  return groups
    .filter((group) => group.entries.length > 0)
    .flatMap((group, index) => {
      const separator: SeparatorEntry = {
        kind: "separator",
        key: `${group.key}-separator`,
      };
      return index === 0 ? [...group.entries] : [separator, ...group.entries];
    });
}

export type KanbanCardMoveTargets = {
  currentWorkflowId: string | null;
  workflowItems: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
};

/** Registry action already bound to this card's immutable task context. */
export type KanbanPluginLinkAction = {
  id: string;
  label: string;
  icon?: PluginIcon;
  disabled?: boolean;
  onSelect: () => void;
};

export type BuildKanbanCardMenuEntriesArgs = {
  currentWorkflowId?: string | null;
  currentStepId?: string | null;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  /** Disables only move and change-workflow entries while a row-local move runs. */
  moveDisabled?: boolean;
  disabled?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isDetaching?: boolean;
  parentTaskId?: string | null;
  /** The task's currently held priority; may be absent, empty or unrecognized. */
  currentPriority?: string | null;
  onSelectPriority?: (priority: TaskPriority) => void;
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
  pluginLinkActions?: KanbanPluginLinkAction[];
  onMoveToStep?: (stepId: string) => void;
  onChangeWorkflow?: () => void;
  onSendToWorkflow?: (workflowId: string, stepId: string) => void;
  isBulkSelection?: boolean;
  /** Defaults to an empty-id context (no visible plugin actions match it in practice). */
  pluginMenuContext?: PluginTaskMenuContext;
  /**
   * Plugin contributions already built from this call's `PluginEntryInputs`.
   * Callers must build them with the same inputs they pass here: the entries
   * carry the disabled state, the edit handler and the menu context, so reusing
   * a set across an input mismatch silently renders one call's plugin entries
   * with another's behaviour. Building them in place is always correct.
   */
  pluginEntries?: CardPluginEntries;
  /**
   * Forces the flat Edit item regardless of registered plugin `edit`-group
   * actions. Group `edit` is a card-only plugin contract; surfaces outside
   * the card set this so they never present the submenu form.
   */
  forceFlatEdit?: boolean;
};

const EMPTY_PLUGIN_MENU_CONTEXT: PluginTaskMenuContext = {
  workspaceId: "",
  taskId: "",
  taskTitle: "",
  workflowStepId: null,
  presentation: "desktop",
};

export function resolvePluginMenuContext(context?: PluginTaskMenuContext): PluginTaskMenuContext {
  return context ?? EMPTY_PLUGIN_MENU_CONTEXT;
}

/** The two plugin-derived pieces of a card menu: group "primary", and `Edit`. */
export type CardPluginEntries = { primary: KanbanCardMenuEntry[]; edit: KanbanCardMenuEntry };

/** The card-menu inputs that decide those two entries. */
export type PluginEntryInputs = Pick<
  BuildKanbanCardMenuEntriesArgs,
  | "disabled"
  | "isDeleting"
  | "isArchiving"
  | "isDetaching"
  | "onEdit"
  | "forceFlatEdit"
  | "pluginMenuContext"
>;

/**
 * Builds both plugin-derived card menu entries. `useKanbanCardMenus` builds one
 * card's dropdown and context variants in a single render and they share every
 * input here, so it calls this once and passes the result to both -- otherwise
 * each plugin action's `items()` runs twice per render for the same children.
 */
export function buildCardPluginEntries(args: PluginEntryInputs): CardPluginEntries {
  const isProcessing = Boolean(
    args.disabled || args.isDeleting || args.isArchiving || args.isDetaching,
  );
  const context = resolvePluginMenuContext(args.pluginMenuContext);
  return {
    primary: buildPrimaryPluginEntries({ disabled: isProcessing, context }),
    edit: buildEditMenuEntry({
      onEdit: args.onEdit,
      disabled: isProcessing,
      context,
      forceFlat: args.forceFlatEdit,
    }),
  };
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

/**
 * Reselecting the task's current priority stays enabled and completes
 * idempotently, unlike `buildStepEntry`, which disables the current step for
 * a move action.
 */
function buildPriorityItemEntry(
  priority: TaskPriority,
  currentPriority: string | null | undefined,
  onSelect: (priority: TaskPriority) => void,
): KanbanCardMenuEntry {
  const isCurrent = isTaskPriority(currentPriority) && currentPriority === priority;
  return {
    kind: "item",
    key: `priority-${priority}`,
    testId: `task-context-priority-${priority}`,
    label: <span className="flex-1 truncate">{t(TASK_PRIORITY_LABEL_KEYS[priority])}</span>,
    trailing: isCurrent ? (
      <span
        data-testid={`task-context-priority-current-${priority}`}
        className="ml-auto text-[10px] text-muted-foreground"
      >
        {t("kanban:current")}
      </span>
    ) : undefined,
    onSelect: () => onSelect(priority),
  };
}

function buildPriorityMenuEntry({
  currentPriority,
  disabled,
  onSelectPriority,
}: {
  currentPriority?: string | null;
  disabled?: boolean;
  onSelectPriority?: (priority: TaskPriority) => void;
}): KanbanCardMenuEntry | null {
  if (!onSelectPriority) return null;
  return {
    kind: "submenu",
    key: "priority",
    testId: "task-context-priority",
    icon: <IconFlag className="mr-2 h-4 w-4" />,
    label: t("kanban:priority"),
    disabled,
    className: "w-40",
    children: TASK_PRIORITY_TOKENS.map((priority) =>
      buildPriorityItemEntry(priority, currentPriority, onSelectPriority),
    ),
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
    key: "change-workflow-selection",
    testId: "task-context-change-workflow-selection",
    icon: <IconLogicBuffer className="mr-2 h-4 w-4" />,
    label: t("task:changeWorkflowForSelectedTasks"),
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

function buildWorkflowMenuEntry({
  currentWorkflowId,
  workflows,
  stepsByWorkflowId,
  disabled,
  onChangeWorkflow,
  onSendToWorkflow,
  isBulkSelection,
}: Pick<
  BuildKanbanCardMenuEntriesArgs,
  | "currentWorkflowId"
  | "workflows"
  | "stepsByWorkflowId"
  | "onChangeWorkflow"
  | "onSendToWorkflow"
  | "isBulkSelection"
> & { disabled: boolean }): KanbanCardMenuEntry | null {
  const visibleWorkflows = workflows.filter((workflow) => !workflow.hidden);
  if (isBulkSelection) {
    return buildSendToWorkflowSubmenu({
      currentWorkflowId,
      workflows: visibleWorkflows,
      stepsByWorkflowId,
      disabled,
      onSendToWorkflow,
    });
  }
  return buildSingleTaskWorkflowEntry({
    currentWorkflowId,
    hasOtherWorkflows: visibleWorkflows.some((workflow) => workflow.id !== currentWorkflowId),
    disabled,
    onChangeWorkflow,
  });
}

function buildCurrentWorkflowMoveEntry(
  currentSteps: TaskMoveStep[],
  currentStepId: string | null | undefined,
  disabled: boolean,
  onMoveToStep: ((stepId: string) => void) | undefined,
) {
  return buildMoveToCurrentWorkflowSubmenu({
    steps: currentSteps,
    currentStepId,
    disabled,
    onMoveToStep,
  });
}

// A flat Edit surface cannot reuse plugin entries that include the card-only Edit group.
function resolveCardPluginEntries({
  pluginEntries,
  forceFlatEdit,
  disabled,
  onEdit,
  pluginMenuContext,
}: Pick<
  BuildKanbanCardMenuEntriesArgs,
  "pluginEntries" | "forceFlatEdit" | "onEdit" | "pluginMenuContext"
> & {
  disabled: boolean;
}): CardPluginEntries {
  if (pluginEntries && !forceFlatEdit) return pluginEntries;
  return buildCardPluginEntries({ disabled, onEdit, forceFlatEdit, pluginMenuContext });
}

export function buildKanbanCardMenuEntries({
  currentWorkflowId,
  currentStepId,
  workflows,
  stepsByWorkflowId,
  moveDisabled,
  disabled,
  isDeleting,
  isArchiving,
  isDetaching,
  parentTaskId,
  currentPriority,
  onSelectPriority,
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
  pluginLinkActions,
  onMoveToStep,
  onChangeWorkflow,
  onSendToWorkflow,
  isBulkSelection,
  pluginMenuContext,
  pluginEntries,
  forceFlatEdit,
}: BuildKanbanCardMenuEntriesArgs): KanbanCardMenuEntry[] {
  const currentSteps = currentWorkflowId ? (stepsByWorkflowId[currentWorkflowId] ?? []) : [];
  const isProcessing = Boolean(disabled || isDeleting || isArchiving || isDetaching);
  const priorityEntry = buildPriorityMenuEntry({
    currentPriority,
    disabled: isProcessing,
    onSelectPriority,
  });

  const moveToEntry = buildCurrentWorkflowMoveEntry(
    currentSteps,
    currentStepId,
    Boolean(moveDisabled || isProcessing),
    onMoveToStep,
  );
  const sendToEntry = buildWorkflowMenuEntry({
    currentWorkflowId,
    workflows,
    stepsByWorkflowId,
    disabled: Boolean(moveDisabled || isProcessing),
    onChangeWorkflow,
    onSendToWorkflow,
    isBulkSelection,
  });

  const pluginContributions = resolveCardPluginEntries({
    pluginEntries,
    forceFlatEdit,
    disabled: isProcessing,
    onEdit,
    pluginMenuContext,
  });

  const linkEntry = buildLinkSubmenu({
    disabled: isProcessing,
    onLinkPullRequest,
    onLinkIssue,
    onLinkMergeRequest,
    onLinkJiraTicket,
    onLinkLinearIssue,
    onLinkSentryIssue,
    pluginLinkActions,
  });

  const detachEntry = buildDetachEntry({ parentTaskId, onDetach, isDetaching, isProcessing });

  return buildGroupedMenuEntries([
    { key: "priority", entries: priorityEntry ? [priorityEntry] : [] },
    {
      key: "edit",
      entries: [pluginContributions.edit],
    },
    {
      key: "relationships",
      entries: [linkEntry, detachEntry].filter(
        (entry): entry is KanbanCardMenuEntry => entry !== null,
      ),
    },
    {
      key: "move",
      entries: [moveToEntry, sendToEntry].filter(
        (entry): entry is KanbanCardMenuEntry => entry !== null,
      ),
    },
    { key: "plugins", entries: pluginContributions.primary },
    {
      key: "remove",
      entries: [
        buildArchiveEntry({ isArchiving, isProcessing, onArchive }),
        buildDeleteEntry({ isDeleting, isProcessing, onDelete }),
      ],
    },
  ]);
}

export function buildArchiveEntry({
  isArchiving,
  isProcessing,
  onArchive,
}: {
  isArchiving?: boolean;
  isProcessing: boolean;
  onArchive?: () => void;
}): KanbanCardMenuEntry {
  return {
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
  };
}

export function buildDeleteEntry({
  isDeleting,
  isProcessing,
  onDelete,
}: {
  isDeleting?: boolean;
  isProcessing: boolean;
  onDelete?: () => void;
}): KanbanCardMenuEntry {
  return {
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
  };
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
