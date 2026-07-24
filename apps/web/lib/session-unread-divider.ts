import type { RenderItem } from "@/hooks/use-processed-messages";

/** Message ids carried by a render item — a "message" item carries its own
 *  id, a "turn_group" carries every message bundled into that visual group,
 *  and non-message items (prepare_progress, agent_error_notice) carry none. */
function renderItemMessageIds(item: RenderItem): string[] {
  if (item.type === "message") return [item.message.id];
  if (item.type === "turn_group") return item.messages.map((message) => message.id);
  return [];
}

/**
 * Locates the Slack-style unread ("New") divider boundary: the key of the
 * render item the divider should appear immediately before.
 *
 * Finds the item containing lastReadMessageId, then returns the key of the
 * next item that carries a real message. When lastReadMessageId falls inside
 * a turn_group (several messages bundled into one visual group), the whole
 * group counts as read and the divider lands after it — a group is never
 * split into read/unread halves.
 *
 * When lastReadMessageId isn't found among the currently loaded items, the
 * loaded window is treated as entirely unread — the divider goes before its
 * first real message — rather than suppressed. The common cause is an older
 * cursor pointing past the currently paginated window (the loaded messages
 * are the transcript's tail; older history hasn't been fetched yet), which
 * means everything visible really is unread. Returns null only when there is
 * no cursor at all (a brand-new session has nothing to mark "New" against),
 * or the cursor already points at the newest loaded message.
 *
 * The returned key mirrors message-list-shared.tsx's getItemKey format
 * (bare message id, or the item's own id for non-message items) — duplicated
 * inline rather than imported to avoid a circular import, since that module
 * imports RenderItem from hooks/use-processed-messages.ts. Keep the two in
 * sync.
 */
export function findUnreadDividerItemId(
  items: RenderItem[],
  lastReadMessageId: string | null | undefined,
): string | null {
  if (!lastReadMessageId) return null;

  const boundaryIndex = items.findIndex((item) =>
    renderItemMessageIds(item).includes(lastReadMessageId),
  );
  const searchFrom = boundaryIndex === -1 ? 0 : boundaryIndex + 1;

  for (let i = searchFrom; i < items.length; i++) {
    const item = items[i];
    if (renderItemMessageIds(item).length > 0) {
      return item.type === "message" ? item.message.id : item.id;
    }
  }
  return null;
}
