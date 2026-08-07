"use client";

import { IconPencil } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import {
  runnablePluginMenuEntry,
  visiblePluginMenuActions,
} from "./kanban-card-plugin-menu-actions";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";

/**
 * Builds the kanban card's "Edit" menu entry. With no plugin action
 * registered for group "edit" this is the flat item exactly as before
 * (AC10). Once a plugin registers one, the pre-existing item becomes
 * "Edit > Edit task" and the plugin's items follow (AC9). An action's
 * `visible(context)` returning false hides it (AC12); a `run` that rejects
 * is caught and logged, and the menu still closes because
 * `DropdownMenuItem`/`ContextMenuItem` already close on select regardless
 * of the async result (AC11).
 */
export function buildEditMenuEntry({
  onEdit,
  disabled,
  context,
}: {
  onEdit?: () => void;
  disabled?: boolean;
  context: PluginTaskMenuContext;
}): KanbanCardMenuEntry {
  const pluginActions = visiblePluginMenuActions("edit", context);

  const icon = <IconPencil className="mr-2 h-4 w-4" />;

  if (pluginActions.length === 0) {
    return {
      kind: "item",
      key: "edit",
      icon,
      label: "Edit",
      disabled: disabled || !onEdit,
      onSelect: onEdit,
    };
  }

  return {
    kind: "submenu",
    key: "edit",
    testId: "kanban-edit-submenu",
    icon,
    label: "Edit",
    disabled,
    children: [
      {
        kind: "item",
        key: "edit-task",
        icon,
        label: t("common:editTask"),
        disabled: disabled || !onEdit,
        onSelect: onEdit,
      },
      ...pluginActions.map((action) => runnablePluginMenuEntry(action, context, disabled)),
    ],
  };
}
