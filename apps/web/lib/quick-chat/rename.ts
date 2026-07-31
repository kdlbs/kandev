import { updateTask } from "@/lib/api/domains/kanban-api";
import { removeStoredQuickChatName, setStoredQuickChatName } from "@/lib/local-storage";

/**
 * Persists a quick-chat tab rename to its backing task title.
 *
 * The title is the shared name: saving it there makes the rename reach every
 * other device (the resulting `task.updated` event re-labels their tab), which
 * a localStorage-only rename never did.
 *
 * localStorage is kept purely as a degraded fallback. On success the local
 * entry is cleared so it can never pin a stale name over a rename made
 * elsewhere; on failure it is written so the user's rename still survives a
 * reload on this device.
 *
 * @returns true when the name reached the backend.
 * @throws the underlying request error, so callers can surface it.
 */
export async function persistQuickChatRename(
  sessionId: string,
  taskId: string | undefined,
  name: string,
): Promise<boolean> {
  if (!sessionId) return false;
  if (!taskId) {
    // An unstarted "New chat" tab has no task yet; keep the name locally.
    setStoredQuickChatName(sessionId, name);
    return false;
  }
  try {
    await updateTask(taskId, { title: name });
    removeStoredQuickChatName(sessionId);
    return true;
  } catch (error) {
    setStoredQuickChatName(sessionId, name);
    throw error;
  }
}

type MigratableSession = { sessionId: string; taskId?: string; name?: string };

/**
 * Uploads renames made before names were persisted server-side.
 *
 * Without this, upgrading would silently revert every tab a user had renamed
 * back to its auto-generated title. Each migrated entry is dropped from
 * localStorage, so this converges after one pass per device.
 */
export async function migrateStoredQuickChatNames(
  sessions: MigratableSession[],
  storedNames: Record<string, string>,
): Promise<void> {
  const pending = sessions.filter(
    (session) =>
      session.taskId &&
      storedNames[session.sessionId] &&
      storedNames[session.sessionId] !== session.name,
  );
  await Promise.all(
    pending.map((session) =>
      // Best effort: a failed migration keeps its local entry and retries on
      // the next resync.
      persistQuickChatRename(
        session.sessionId,
        session.taskId,
        storedNames[session.sessionId],
      ).catch(() => undefined),
    ),
  );
}
