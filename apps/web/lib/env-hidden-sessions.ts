import { getSessionStorage, setSessionStorage } from "@/lib/local-storage";

const prefix = "kandev.dockview.env-hidden-sessions-v1.";

export type EnvHiddenSessionRecord = { sessionId: string; taskId: string };

function read(envId: string): EnvHiddenSessionRecord[] {
  return getSessionStorage<string[]>(`${prefix}${envId}`, []).map((value) => {
    const separator = value.lastIndexOf("@");
    return separator < 0
      ? { sessionId: value, taskId: "" }
      : { sessionId: value.slice(0, separator), taskId: value.slice(separator + 1) };
  });
}

export function getEnvHiddenSessions(envId: string): string[] {
  return read(envId).map((record) => record.sessionId);
}

export function getEnvHiddenSessionRecords(envId: string): EnvHiddenSessionRecord[] {
  return read(envId);
}

export function setEnvHiddenSessionOwner(envId: string, sessionId: string, taskId: string): void {
  const records = read(envId).filter((record) => record.sessionId !== sessionId);
  records.push({ sessionId, taskId });
  setSessionStorage(
    `${prefix}${envId}`,
    records.map((record) => `${record.sessionId}@${record.taskId}`),
  );
}

export function setEnvHiddenSessions(envId: string, sessionIds: string[]): void {
  const owners = new Map(read(envId).map((record) => [record.sessionId, record.taskId]));
  setSessionStorage(
    `${prefix}${envId}`,
    sessionIds.map((id) => `${id}@${owners.get(id) ?? ""}`),
  );
}

/** Keep implicit selection on a visible session without clearing explicit hide records. */
export function resolveVisibleSessionId(
  activeSessionId: string | null,
  sessionIds: readonly string[],
  hiddenSessionIds: Iterable<string>,
): string | null {
  if (!activeSessionId) return null;
  const hidden = new Set(hiddenSessionIds);
  return hidden.has(activeSessionId)
    ? (sessionIds.find((sessionId) => !hidden.has(sessionId)) ?? null)
    : activeSessionId;
}
