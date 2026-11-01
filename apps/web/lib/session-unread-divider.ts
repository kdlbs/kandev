import type { RenderItem } from "@/hooks/use-processed-messages";
import { isFooterActionMessage } from "@/hooks/processed-message-filtering";
import type { Message } from "@/lib/types/http";

/** The frontend-only placeholder use-processed-messages.ts injects for a
 *  session with no real messages. It never exists as a backend row, so it can
 *  be neither a read cursor nor a divider boundary. */
const TASK_DESCRIPTION_PLACEHOLDER_ID = "task-description";

/** Message ids carried by a render item — a "message" item carries its own
 *  id, a "turn_group" carries every message bundled into that visual group,
 *  and non-message items (prepare_progress, agent_error_notice) carry none.
 *  The task-description placeholder is excluded. */
function renderItemMessageIds(item: RenderItem): string[] {
  if (item.type === "message") {
    return item.message.id === TASK_DESCRIPTION_PLACEHOLDER_ID ? [] : [item.message.id];
  }
  if (item.type === "turn_group") {
    return item.messages
      .map((message) => message.id)
      .filter((id) => id !== TASK_DESCRIPTION_PLACEHOLDER_ID);
  }
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

/**
 * Returns the id the read cursor acknowledges for a displayed session: the
 * newest message the client has loaded, whether or not the transcript renders
 * it. Every message the server can count as unread is stored and loaded
 * before it can render, so acknowledging the loaded tail always reads past
 * hidden or collapsed rows; acknowledging only the rendered tail could leave
 * such a row unread forever. Footer-action notices are skipped: they never
 * count as unread, and a transient-retry notice is deleted when the retry ends,
 * which would leave a cursor with no position in the session.
 */
export function newestAcknowledgeableMessageId(messages: Message[]): string | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i];
    if (message.id !== TASK_DESCRIPTION_PLACEHOLDER_ID && !isFooterActionMessage(message)) {
      return message.id;
    }
  }
  return null;
}

/**
 * Maps a read cursor onto the rendered transcript before divider placement.
 * The cursor can point at a loaded message the transcript does not render;
 * the divider then belongs after the newest rendered message at or before it.
 * A cursor with nothing rendered before it, or outside the loaded window,
 * passes through so findUnreadDividerItemId treats the window as unread.
 */
export function dividerCursorForRenderedItems(
  messages: Message[],
  items: RenderItem[],
  cursor: string | null | undefined,
): string | null {
  if (!cursor) return null;
  const rendered = new Set(items.flatMap(renderItemMessageIds));
  if (rendered.has(cursor)) return cursor;
  const cursorIndex = messages.findIndex((message) => message.id === cursor);
  for (let i = cursorIndex - 1; i >= 0; i--) {
    if (rendered.has(messages[i].id)) return messages[i].id;
  }
  return cursor;
}
