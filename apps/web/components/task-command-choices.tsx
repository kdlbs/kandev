import { createElement } from "react";
import { IconLogicBuffer } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { CommandItem } from "@/lib/commands/types";
import { useSetTaskColor, useTaskColor } from "@/hooks/use-task-color";
import { useUpdateTaskPriority } from "@/hooks/use-update-task-priority";
import { useNestTask } from "@/hooks/use-nest-task";
import { useAppStore } from "./state-provider";
import { TASK_COLORS, TASK_COLOR_LABEL_KEYS, TASK_COLOR_BAR_CLASS } from "@/lib/task-colors";
import { TASK_PRIORITY_TOKENS, TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
import { computeNestCandidates } from "@/lib/sidebar/nest-candidates";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import {
  taskMoveOptions,
  type TaskMoveStep,
  type TaskMoveWorkflow,
} from "./task/task-move-context-menu";
import { useTaskPluginLinkActions } from "./task/task-session-sidebar-link-actions";
import { useTaskPluginPrimaryMenuEntries } from "./task/task-switcher-plugin-menu-items";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";
import {
  selectTaskLinkActions,
  taskLinkMenuOptions,
  type TaskLinkHandlers,
} from "./task/task-switcher-link-menu";
import type { TaskSwitcherItem } from "./task/task-switcher-types";

const noop = () => {};

type ChoiceOptions = {
  task: TaskSwitcherItem;
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, TaskMoveStep[]>;
  linkHandlers: TaskLinkHandlers;
  openMoveOptions: (step: TaskMoveStep) => void;
  moveImmediately: (step: TaskMoveStep) => void;
  onChangeWorkflow?: () => void;
};

/**
 * One command per *selectable* plugin menu entry. A plugin submenu has no
 * activation of its own (its `label` is only a trigger and it carries no
 * `onSelect`), so its item children become the commands -- flattening them
 * keeps a plugin's actions reachable from the command palette instead of
 * dropping the whole contribution, and mirrors what the search surface shows
 * for every other card action.
 */
export function pluginCommandChoices(
  entry: KanbanCardMenuEntry,
  group: string,
  parentLabel?: string,
): CommandItem[] {
  if (entry.kind === "submenu") {
    const label = typeof entry.label === "string" ? entry.label : parentLabel;
    return entry.children.flatMap((child) => pluginCommandChoices(child, group, label));
  }
  if (entry.kind !== "item" || typeof entry.label !== "string") return [];
  return [
    {
      // The entry key is unique across every plugin menu entry, including
      // flattened children whose key encodes the child id, and it doubles as
      // this row's React key and cmdk value.
      id: entry.key,
      label: entry.label,
      group,
      // A child's own label ("Blocked") is meaningless on its own in a palette
      // listing every action, so the trigger it came from is the entry's
      // context -- the same field the sidebar task commands use for the title.
      context: parentLabel,
      action: entry.onSelect,
      disabled: entry.disabled,
      icon: entry.icon,
    },
  ];
}

export function useTaskCommandChoices(options: ChoiceOptions) {
  const { task, linkHandlers } = options;
  const { t } = useTranslation();
  const group = t("common:commandGroupTasks");
  const setColor = useSetTaskColor();
  const currentColor = useTaskColor(task.id);
  const updatePriority = useUpdateTaskPriority();
  const pluginLinks = useTaskPluginLinkActions(task.id, task.repositoryLinks ?? []);
  const pluginEntries = useTaskPluginPrimaryMenuEntries(task);
  const item = (id: string, label: string, action?: () => void): CommandItem => ({
    id,
    label,
    group,
    action,
  });
  const colors: CommandItem[] = TASK_COLORS.map((color) => ({
    ...item(`task-color-${color}`, t(TASK_COLOR_LABEL_KEYS[color]), () => setColor(task.id, color)),
    icon: (
      <span aria-hidden className={`block size-2.5 rounded-full ${TASK_COLOR_BAR_CLASS[color]}`} />
    ),
  }));
  colors.push({
    ...item("task-color-none", t("task:groupNone"), () => setColor(task.id, null)),
    disabled: !currentColor,
  });
  if (task.automaticColorSource)
    colors.unshift({
      ...item(
        "task-color-source",
        t("task:automaticColorSource", { rule: task.automaticColorSource.label }),
      ),
      disabled: true,
    });
  const priorities = TASK_PRIORITY_TOKENS.map((priority) => ({
    ...item(
      `task-priority-${priority}`,
      t(TASK_PRIORITY_LABEL_KEYS[priority]),
      () => void updatePriority(task.id, priority),
    ),
    context: task.priority === priority ? t("kanban:current") : undefined,
  }));
  const links = taskLinkMenuOptions(selectTaskLinkActions(task, noop, linkHandlers)).map(
    ({ id, labelKey, Icon, onSelect }) => ({
      ...item(`task-link-${id}`, t(labelKey), onSelect),
      icon: <Icon className="size-3.5" />,
    }),
  );
  links.push(
    ...pluginLinks.map((link) => ({
      ...item(`task-link-plugin-${link.id}`, link.label, link.onSelect),
      icon: createElement(resolvePluginIcon(link.icon), { className: "size-3.5" }),
    })),
  );
  const plugins = pluginEntries.flatMap((entry) => pluginCommandChoices(entry, group));
  return {
    colors,
    priorities,
    links,
    plugins,
    nesting: useTaskNestChoices(task),
    ...useTaskMoveChoices(options),
  };
}

function useTaskNestChoices(task: TaskSwitcherItem): CommandItem[] {
  const { t } = useTranslation();
  const nestTask = useNestTask();
  const tasks = useAppStore((state) =>
    task.workflowId
      ? (state.kanbanMulti.snapshots[task.workflowId]?.tasks ??
        (state.kanban.workflowId === task.workflowId ? state.kanban.tasks : undefined))
      : undefined,
  );
  const group = t("task:nestUnder");
  const candidates = computeNestCandidates(tasks ?? [], task.id);
  const items: CommandItem[] = candidates.map((candidate) => ({
    id: `task-nest-${candidate.id}`,
    label: candidate.title,
    group,
    action: () => {
      if (task.workflowId) void nestTask(task.id, task.workflowId, candidate.id);
    },
  }));
  if (!items.length)
    items.push({ id: "task-nest-empty", label: t("task:noOtherTasks"), group, disabled: true });
  if (task.parentTaskId)
    items.unshift({
      id: "task-unnest",
      label: t("task:unNestRemoveParent"),
      group,
      action: () => {
        if (task.workflowId) void nestTask(task.id, task.workflowId, null);
      },
    });
  return items;
}

export function useTaskMoveChoices({
  task,
  workflows,
  stepsByWorkflowId,
  openMoveOptions,
  moveImmediately,
  onChangeWorkflow,
}: Omit<ChoiceOptions, "linkHandlers">) {
  const { t } = useTranslation();
  const { currentSteps, targets } = taskMoveOptions(task.workflowId, workflows, stepsByWorkflowId);
  const group = t("task:moveTo");
  const choice = (step: TaskMoveStep, workflowId: string, group: string): CommandItem => {
    const target = { ...step, workflow_id: workflowId };
    return {
      id: `task-move-${workflowId}-${step.id}`,
      label: step.title,
      group,
      icon: (
        <span
          aria-hidden
          className={`block size-2.5 rounded-full ${step.color || "bg-slate-500"}`}
        />
      ),
      action: () => openMoveOptions(target),
      immediateAction: () => moveImmediately(target),
    };
  };
  const steps = currentSteps
    .filter((step) => step.id !== task.workflowStepId)
    .map((step) => choice(step, task.workflowId!, group));
  const workflowChoices: CommandItem[] = targets.length
    ? [
        {
          id: "task-change-workflow",
          label: t("task:changeWorkflow"),
          group,
          icon: <IconLogicBuffer className="size-3.5" />,
          action: onChangeWorkflow,
        },
      ]
    : [];
  return { steps, workflows: workflowChoices };
}
