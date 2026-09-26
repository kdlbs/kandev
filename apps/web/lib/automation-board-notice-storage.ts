import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";

/**
 * Durable dismissal for the one-time "automation runs moved off the board"
 * notice.
 *
 * Per workspace, not per user or per automation: the thing that changed is
 * where a workspace's automation runs show up, so once someone has read it for
 * a workspace there is nothing left to say about that workspace — but a
 * second workspace they have not opened yet still has the news to break.
 *
 * localStorage rather than the server: the notice is advice about a change
 * that already happened, and losing the dismissal on a new browser costs the
 * reader one more sighting of a sentence they have already read. Persisting it
 * server-side would mean a schema and an endpoint for a migration window that
 * closes once.
 *
 * The storage primitives are the shared ones in `lib/local-storage` — SSR
 * guard and try/catch included — so this is the same dismissal mechanism the
 * walkthrough notification uses, not a second one.
 */
const DISMISSED_KEY = "kandev.automations.boardMoveNoticeDismissed";

type DismissedByWorkspace = Record<string, boolean>;

export function isBoardMoveNoticeDismissed(workspaceId: string): boolean {
  return getLocalStorage<DismissedByWorkspace>(DISMISSED_KEY, {})[workspaceId] === true;
}

export function dismissBoardMoveNotice(workspaceId: string): void {
  const dismissed = getLocalStorage<DismissedByWorkspace>(DISMISSED_KEY, {});
  setLocalStorage(DISMISSED_KEY, { ...dismissed, [workspaceId]: true });
}
