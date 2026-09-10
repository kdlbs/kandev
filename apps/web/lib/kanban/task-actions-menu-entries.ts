import {
  buildArchiveEntry,
  buildDeleteEntry,
  buildKanbanCardMenuEntries,
  resolvePluginMenuContext,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";
import { buildPrimaryPluginEntries } from "@/components/plugins/task-menu-actions";
import type { BuildKanbanCardMenuEntriesArgs } from "@/components/kanban-card-menu-items";

/**
 * `normal`: the subject is not archived and its board row is resolved, so
 * the full card entry set applies. `archived`: the board excludes archived
 * tasks, so this is a detail-surface-only branch the card has none to
 * inherit from. `unresolved-row`: the subject's board row cannot be found,
 * so only identifier-only entries are available.
 */
export type TaskActionsMenuTier = "normal" | "archived" | "unresolved-row";

export function buildTaskActionsMenuEntries(
  tier: TaskActionsMenuTier,
  args: BuildKanbanCardMenuEntriesArgs,
): KanbanCardMenuEntry[] {
  if (tier === "archived") return buildArchivedTaskActionsMenuEntries(args);
  if (tier === "unresolved-row") return buildUnresolvedRowTaskActionsMenuEntries(args);
  return buildKanbanCardMenuEntries({ ...args, forceFlatEdit: true });
}

function buildArchivedTaskActionsMenuEntries({
  disabled,
  isDeleting,
  onDelete,
  pluginMenuContext,
}: BuildKanbanCardMenuEntriesArgs): KanbanCardMenuEntry[] {
  const isProcessing = Boolean(disabled || isDeleting);
  const pluginEntries = buildPrimaryPluginEntries({
    disabled: isProcessing,
    context: resolvePluginMenuContext(pluginMenuContext),
  });
  const entries: KanbanCardMenuEntry[] = [...pluginEntries];
  if (pluginEntries.length > 0) {
    entries.push({ kind: "separator", key: "delete-separator" });
  }
  entries.push(buildDeleteEntry({ isDeleting, isProcessing, onDelete }));
  return entries;
}

function buildUnresolvedRowTaskActionsMenuEntries({
  disabled,
  isDeleting,
  isArchiving,
  onArchive,
  onDelete,
  pluginMenuContext,
}: BuildKanbanCardMenuEntriesArgs): KanbanCardMenuEntry[] {
  const isProcessing = Boolean(disabled || isDeleting || isArchiving);
  return [
    ...buildPrimaryPluginEntries({
      disabled: isProcessing,
      context: resolvePluginMenuContext(pluginMenuContext),
    }),
    buildArchiveEntry({ isArchiving, isProcessing, onArchive }),
    { kind: "separator", key: "delete-separator" },
    buildDeleteEntry({ isDeleting, isProcessing, onDelete }),
  ];
}
