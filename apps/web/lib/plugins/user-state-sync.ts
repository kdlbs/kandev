/**
 * Implements `host.storage.subscribe` (approach F1,
 * docs/plans/plugins/PLUGIN-API.md): a thin, typed wrapper over
 * `registerWsHandler("plugin.user-state.updated", …)` with own-plugin
 * filtering, scope/scopeId/key filtering, and own-tab echo suppression.
 */
import { pluginRegistry } from "./registry";
import type { PluginStorageScope, PluginUserStateChange } from "./types";

/** Must match `ActionPluginUserStateUpdated` in apps/backend/pkg/websocket/actions.go. */
export const PLUGIN_USER_STATE_UPDATED_ACTION = "plugin.user-state.updated";

/** Keep writer ids identical on both the storage write and subscribe paths. */
export function composeWriterId(baseWriterId: string, surfaceId: string | undefined): string {
  return surfaceId ? `${baseWriterId}:${surfaceId}` : baseWriterId;
}

export interface UserStateSubscribeFilter {
  scope?: PluginStorageScope;
  scopeId?: string;
  key?: string;
  /** Per-surface discriminator combined with localWriterId — see PluginStorageApi.subscribe. */
  writerId?: string;
}

/** Shape of the WS notification payload published by the backend on write/delete. */
interface UserStateUpdatedPayload {
  pluginId?: string;
  scope?: PluginStorageScope;
  scopeId?: string;
  key?: string;
  updatedAt?: string;
  writerId?: string;
  deleted?: boolean;
}

function matchesFilter(filter: UserStateSubscribeFilter, change: PluginUserStateChange): boolean {
  if (filter.scope && filter.scope !== change.scope) return false;
  if (filter.scopeId && filter.scopeId !== change.scopeId) return false;
  if (filter.key && filter.key !== change.key) return false;
  return true;
}

/**
 * Subscribes `pluginId` to live per-user storage updates. `localWriterId` is
 * the default writer id this browser tab stamps on its own writes (see
 * `host-api.ts`'s TAB_WRITER_ID) — a notification carrying the same writerId
 * is treated as an echo and is skipped (AC25) so an editor never clobbers
 * its own caret/selection from its own write.
 *
 * `filter.writerId`, if given, is appended to `localWriterId` (not a
 * replacement) to form the actual comparator — `${localWriterId}:${filter.writerId}`.
 * Appending rather than replacing matters: a surface id such as a dockview
 * `panelId` is a static string shared by every browser tab that has that
 * panel open, so using it alone would make two different tabs editing the
 * same document look like the same writer to each other and break cross-tab
 * sync (AC24). Combining it with the per-tab `localWriterId` keeps both
 * properties: different tabs always differ, and — when a plugin gives each
 * of its surfaces (e.g. an open task panel and a kanban quick-action) its
 * own distinct writerId, on both this filter and the matching `set`/`delete`
 * calls that surface makes — different surfaces in the same tab always
 * differ too, without which every surface shares the single tab-wide
 * default and each one's writes look like every other surface's own echo,
 * silently swallowing legitimate cross-surface sync.
 *
 * Registers one WS handler per call (via `registerWsHandler`), so the
 * returned unsubscribe can remove exactly this subscription
 * (`unregisterWsHandler`) without disturbing any other subscription the
 * plugin holds, while a full plugin disable/uninstall still bulk-revokes it
 * via the existing `unregisterPlugin` path (AC26).
 */
export function subscribeToUserStateChanges(
  pluginId: string,
  localWriterId: string,
  filter: UserStateSubscribeFilter,
  handler: (change: PluginUserStateChange) => void,
): () => void {
  const ownWriterId = composeWriterId(localWriterId, filter.writerId);
  const wsHandler = (payload: unknown): void => {
    const raw = payload as UserStateUpdatedPayload | undefined;
    if (!raw || raw.pluginId !== pluginId) return;
    if (raw.writerId && raw.writerId === ownWriterId) return;

    const change: PluginUserStateChange = {
      scope: (raw.scope ?? "instance") as PluginStorageScope,
      scopeId: raw.scopeId ?? "",
      key: raw.key ?? "",
      updatedAt: raw.updatedAt ?? "",
      deleted: raw.deleted,
    };
    if (!matchesFilter(filter, change)) return;
    handler(change);
  };

  pluginRegistry.registerWsHandler(pluginId, PLUGIN_USER_STATE_UPDATED_ACTION, wsHandler);
  return () => {
    pluginRegistry.unregisterWsHandler(pluginId, PLUGIN_USER_STATE_UPDATED_ACTION, wsHandler);
  };
}
