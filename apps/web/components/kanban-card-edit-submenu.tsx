"use client";

import { IconLoader2, IconPencil } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { pluginMenuEntry, visiblePluginMenuActions } from "./plugins/task-menu-actions";
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
  forceFlat,
  nativeUnlinkEntries = [],
  loadingUnlinkLabel,
}: {
  onEdit?: () => void;
  disabled?: boolean;
  context: PluginTaskMenuContext;
  /** Skip the plugin `edit`-group lookup and always return the flat item. */
  forceFlat?: boolean;
  nativeUnlinkEntries?: KanbanCardMenuEntry[];
  loadingUnlinkLabel?: string;
}): KanbanCardMenuEntry {
  const pluginActions = forceFlat ? [] : visiblePluginMenuActions("edit", context);
  // Built and filtered before choosing between the submenu and the flat item: an
  // action the host cannot render contributes no child, so a list yielding none
  // must leave the native Edit item as it is rather than wrap it in a submenu.
  const pluginEntries = pluginActions
    .map((action) => pluginMenuEntry(action, context, disabled))
    .filter((entry): entry is KanbanCardMenuEntry => entry !== null);

  const icon = <IconPencil className="mr-2 h-4 w-4" />;

  if (pluginEntries.length === 0 && nativeUnlinkEntries.length === 0 && !loadingUnlinkLabel) {
    return {
      kind: "item",
      key: "edit",
      icon,
      label: t("common:edit"),
      disabled: disabled || !onEdit,
      onSelect: onEdit,
    };
  }

  return {
    kind: "submenu",
    key: "edit",
    testId: "kanban-edit-submenu",
    icon,
    label: t("common:edit"),
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
      ...nativeUnlinkEntries,
      ...(loadingUnlinkLabel
        ? [
            {
              kind: "item" as const,
              key: "loading-pull-requests",
              testId: "kanban-edit-loading-pull-requests",
              icon: <IconLoader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />,
              label: loadingUnlinkLabel,
              disabled: true,
            },
          ]
        : []),
      ...pluginEntries,
    ],
  };
}
