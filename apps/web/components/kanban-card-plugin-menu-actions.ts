import { pluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";
type PluginTaskMenuAction = ReturnType<typeof pluginRegistry.getTaskMenuActions>[number];

/**
 * Registered actions for `group`, filtered by `visible(context)` (default
 * visible when omitted). A `visible` that throws is caught, logged, and
 * treated as hidden — the same defensive handling as `run`'s rejection.
 */
export function visiblePluginMenuActions(
  group: PluginTaskMenuAction["group"],
  context: PluginTaskMenuContext,
): PluginTaskMenuAction[] {
  return pluginRegistry.getTaskMenuActions(group).filter((action) => {
    if (!action.visible) return true;
    try {
      return action.visible(context);
    } catch (error: unknown) {
      console.error(
        `[plugins] task menu action "${action.pluginId}:${action.id}" visible() threw`,
        error,
      );
      return false;
    }
  });
}

/**
 * Builds a runnable menu entry for a plugin task menu action. A `run` that
 * throws or rejects is caught and logged; the menu still closes because
 * `DropdownMenuItem`/`ContextMenuItem` already close on select regardless
 * of the async result.
 */
export function runnablePluginMenuEntry(
  action: PluginTaskMenuAction,
  context: PluginTaskMenuContext,
  disabled?: boolean,
  keyPrefix = "plugin-edit",
): KanbanCardMenuEntry {
  return {
    kind: "item",
    key: `${keyPrefix}-${action.pluginId}-${action.id}`,
    icon: action.icon,
    label: action.label,
    disabled,
    onSelect: () => {
      // Promise.resolve().then(() => action.run(context)) — not
      // Promise.resolve(action.run(context)) — so a *synchronous* throw
      // inside run() also lands in the .catch() below. Calling action.run
      // directly as the Promise.resolve() argument still throws before that
      // expression finishes evaluating, escaping past .catch() entirely and
      // straight out of this onSelect handler.
      Promise.resolve()
        .then(() => action.run(context))
        .catch((error: unknown) => {
          console.error(
            `[plugins] task menu action "${action.pluginId}:${action.id}" failed`,
            error,
          );
        });
    },
  };
}

/**
 * Builds flat, top-level menu entries for group "primary" task menu
 * actions, in registration order. Unlike group "edit" (nested in the Edit
 * submenu), these render directly in the card menu — positioned between
 * "Move to"/"Send to workflow" and "Link".
 */
export function buildPrimaryPluginEntries({
  disabled,
  context,
}: {
  disabled?: boolean;
  context: PluginTaskMenuContext;
}): KanbanCardMenuEntry[] {
  return visiblePluginMenuActions("primary", context).map((action) =>
    runnablePluginMenuEntry(action, context, disabled, "plugin-primary"),
  );
}
