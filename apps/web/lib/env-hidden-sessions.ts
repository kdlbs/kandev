import { getSessionStorage, setSessionStorage } from "@/lib/local-storage";

// Session panels a user explicitly closed (panel-only hide) for a task
// environment, attributed to the task that owns each hidden session. The
// dockview layout blob cannot carry this: reusable layouts normalize session
// panels to chat placeholders, so the hide record lives beside the env layout
// with the same per-env, per-browser-tab lifetime. Owners let pruning wait for
// each owning task's session list to hydrate instead of trusting one active
// task's list for the whole shared environment.
const DOCKVIEW_ENV_HIDDEN_SESSIONS_PREFIX = "kandev.dockview.env-hidden-sessions-v1.";

export type EnvHiddenSessionRecord = { sessionId: string; taskId: string };

function splitRecord(value: string): [string, string] {
  // v1 records are bare session IDs; attributed records are "sessionId@taskId".
  const separator = value.lastIndexOf("@");
  if (separator === -1) return [value, ""];
  return [value.slice(0, separator), value.slice(separator + 1)];
}

function readRecords(envId: string): EnvHiddenSessionRecord[] {
  const persisted = getSessionStorage<string[]>(
    `${DOCKVIEW_ENV_HIDDEN_SESSIONS_PREFIX}${envId}`,
    [],
  );
  return persisted.map((value) => {
    const [sessionId, taskId] = splitRecord(value);
    return { sessionId, taskId };
  });
}

function writeRecords(envId: string, records: EnvHiddenSessionRecord[]): void {
  setSessionStorage(
    `${DOCKVIEW_ENV_HIDDEN_SESSIONS_PREFIX}${envId}`,
    records.map((record) =>
      record.taskId ? `${record.sessionId}@${record.taskId}` : record.sessionId,
    ),
  );
}

/** Read the per-env session IDs whose panels were explicitly closed. */
export function getEnvHiddenSessions(envId: string): string[] {
  return readRecords(envId).map((record) => record.sessionId);
}

/** Persist the per-env set of explicitly-closed session panels. */
export function setEnvHiddenSessions(envId: string, sessionIds: string[]): void {
  // Preserve each entry's existing owner attribution; entries the caller has
  // not attributed remain ownerless rather than being pinned to a wrong task.
  const previous = new Map(readRecords(envId).map((record) => [record.sessionId, record.taskId]));
  writeRecords(
    envId,
    sessionIds.map((sessionId) => ({ sessionId, taskId: previous.get(sessionId) ?? "" })),
  );
}

/** Attribute an explicitly closed session panel to the task that owns it. */
export function setEnvHiddenSessionOwner(envId: string, sessionId: string, taskId: string): void {
  const records = readRecords(envId).filter((record) => record.sessionId !== sessionId);
  records.push({ sessionId, taskId });
  writeRecords(envId, records);
}

/** Read the per-env hidden records with their owning task attribution. */
export function getEnvHiddenSessionRecords(envId: string): EnvHiddenSessionRecord[] {
  return readRecords(envId);
}
