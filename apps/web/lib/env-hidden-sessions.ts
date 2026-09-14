import { getSessionStorage, setSessionStorage } from "@/lib/local-storage";

// Session panels a user explicitly closed (panel-only hide) for a task
// environment. The dockview layout blob cannot carry this: reusable layouts
// normalize session panels to chat placeholders, so the hide record lives
// beside the env layout with the same per-env, per-browser-tab lifetime.
const DOCKVIEW_ENV_HIDDEN_SESSIONS_PREFIX = "kandev.dockview.env-hidden-sessions-v1.";

/** Read the per-env session IDs whose panels were explicitly closed. */
export function getEnvHiddenSessions(envId: string): string[] {
  return getSessionStorage(`${DOCKVIEW_ENV_HIDDEN_SESSIONS_PREFIX}${envId}`, []);
}

/** Persist the per-env set of explicitly-closed session panels. */
export function setEnvHiddenSessions(envId: string, sessionIds: string[]): void {
  setSessionStorage(`${DOCKVIEW_ENV_HIDDEN_SESSIONS_PREFIX}${envId}`, sessionIds);
}
