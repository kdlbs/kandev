import type { TaskSession } from "@/lib/types/http";
import { migrateEnvKeyedData } from "@/lib/state/slices/session-runtime/session-runtime-slice";
import { prepareResultToSessionState } from "@/lib/state/slices/session-runtime/prepare-result";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debugEnv = createDebugLogger("session:env-mapping");

/** Eagerly populate session→environment mapping and migrate any data stored under the fallback key.
 *  `draft` must be the combined store state (SessionSlice + SessionRuntimeSlice). */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function syncEnvironmentMapping(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  draft: any,
  sessionId: string,
  environmentId: string | undefined,
) {
  if (!environmentId) return;
  const previous = draft.environmentIdBySessionId[sessionId];
  if (isDebug()) {
    debugEnv("syncEnvironmentMapping", {
      sessionId,
      environmentId,
      previous: previous ?? null,
      changed: previous !== environmentId,
      fallbackGitStatusFileCount: Object.keys(
        draft.gitStatus?.byEnvironmentId?.[sessionId]?.files ?? {},
      ).length,
      targetGitStatusFileCount: Object.keys(
        draft.gitStatus?.byEnvironmentId?.[environmentId]?.files ?? {},
      ).length,
    });
  }
  draft.environmentIdBySessionId[sessionId] = environmentId;
  migrateEnvKeyedData(draft, sessionId, environmentId);
}

/**
 * Backfill the prepare-progress slice from a session's `metadata.prepare_result`
 * when sessions are loaded from the API (e.g. switching tasks client-side).
 *
 * Without this, prepare progress only ever arrives via SSR hydration or live WS
 * events, so switching to a task whose prepare already completed (common for
 * remote executors) showed an empty "Environment prepared" row until a full
 * page reload re-ran SSR. Only populates when no entry exists yet so we never
 * clobber live WS progress for an in-flight prepare.
 *
 * `draft` must be the combined store state (SessionSlice + SessionRuntimeSlice).
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function syncPrepareProgress(draft: any, session: TaskSession) {
  if (draft.prepareProgress.bySessionId[session.id]) return;
  const prepareState = prepareResultToSessionState(session.id, session.metadata);
  if (prepareState) draft.prepareProgress.bySessionId[session.id] = prepareState;
}
