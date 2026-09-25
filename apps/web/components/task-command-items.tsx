import {
  IconPin,
  IconPinFilled,
  IconPalette,
  IconFlag,
  IconEdit,
  IconPencil,
  IconCopy,
  IconSubtask,
  IconLink,
  IconUnlink,
  IconArrowRight,
  IconLogicBuffer,
  IconTrash,
} from "@tabler/icons-react";
import { taskRowActionAvailability } from "./task/task-row-action-availability";
import type { CommandItem } from "@/lib/commands/types";
import type { TaskSwitcherItem } from "./task/task-switcher-types";
import type { TFunction } from "i18next";
export type TaskCommandContext = {
  task: TaskSwitcherItem;
  t: TFunction;
  isPinned: boolean;
  disabled?: boolean;
  colors: CommandItem[];
  priorities: CommandItem[];
  links: CommandItem[];
  nesting: CommandItem[];
  steps: CommandItem[];
  workflows: CommandItem[];
  plugins: CommandItem[];
  onPin: () => void;
  onEdit: () => void;
  onRename: () => void;
  onDetach: () => void;
  onDelete: () => void;
};
export function buildSidebarTaskCommands(ctx: TaskCommandContext): CommandItem[] {
  const { task, t } = ctx;
  const available = taskRowActionAvailability(task);
  const group = t("common:commandGroupTasks");
  const command = (
    id: string,
    label: string,
    Icon: typeof IconPin,
    action?: () => void,
  ): CommandItem => ({
    id,
    label,
    group,
    icon: <Icon className="size-3.5" />,
    action,
    disabled: ctx.disabled,
  });
  const items = [
    command(
      "task-pin",
      t(ctx.isPinned ? "task:unpin" : "task:annotationPin"),
      ctx.isPinned ? IconPinFilled : IconPin,
      ctx.onPin,
    ),
  ];
  const choices = (id: string, label: string, Icon: typeof IconPin, children: CommandItem[]) => {
    if (children.length) items.push({ ...command(id, label, Icon), children });
  };
  if (available.mark) {
    choices("task-color", t("task:color"), IconPalette, ctx.colors);
    choices("task-priority", t("kanban:priority"), IconFlag, ctx.priorities);
  }
  if (available.edit) items.push(command("task-edit", t("common:edit"), IconEdit, ctx.onEdit));
  items.push(command("task-rename", t("task:rename"), IconPencil, ctx.onRename));
  if (available.mark)
    items.push({ ...command("task-duplicate", t("settings:duplicate"), IconCopy), disabled: true });
  if (available.nest) choices("task-nest", t("task:nestUnder"), IconSubtask, ctx.nesting);
  choices("task-link", t("task:link"), IconLink, ctx.links);
  if (available.detach)
    items.push(command("task-detach", t("task:detachFromParent"), IconUnlink, ctx.onDetach));
  if (available.move) {
    choices("task-move", t("task:moveTo"), IconArrowRight, ctx.steps);
    choices("task-send-workflow", t("task:sendToWorkflow"), IconLogicBuffer, ctx.workflows);
  }
  items.push(...ctx.plugins);
  items.push({
    ...command("task-delete", t("task:delete"), IconTrash, ctx.onDelete),
    destructive: true,
  });
  return items.map((item) => ({
    ...item,
    priority: 0,
    disabled: item.disabled || ctx.disabled,
    // An item that names its own context keeps it (a plugin submenu child
    // carries the trigger it came from); the task title is only the default
    // second line for everything else.
    context: item.context ?? task.title,
  }));
}
