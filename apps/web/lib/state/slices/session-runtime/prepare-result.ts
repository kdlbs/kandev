import type { SessionPrepareState } from "./types";

/**
 * Raw `prepare_result` shape as stored in a session's `metadata` (snake_case,
 * straight off the backend). Mapped into the camelCase {@link SessionPrepareState}
 * the store and UI consume.
 */
type RawPrepareStep = {
  name: string;
  command?: string;
  status: string;
  output?: string;
  error?: string;
  warning?: string;
  warning_detail?: string;
  started_at?: string;
  ended_at?: string;
};

type RawPrepareResult = {
  status?: string;
  steps?: RawPrepareStep[];
  error_message?: string;
  duration_ms?: number;
};

/**
 * Map a session's `metadata.prepare_result` into the store's
 * {@link SessionPrepareState}. Returns `null` when the session has no prepare
 * result (nothing to render).
 *
 * Used by both the SSR page-state builder and the client-side session loader so
 * the "Environment prepared" timeline message is populated the same way whether
 * the page is freshly loaded or the user switches tasks client-side.
 */
export function prepareResultToSessionState(
  sessionId: string,
  metadata: Record<string, unknown> | null | undefined,
): SessionPrepareState | null {
  const pr = metadata?.prepare_result as RawPrepareResult | undefined;
  if (!pr) return null;

  return {
    sessionId,
    status: pr.status ?? "completed",
    steps: (pr.steps ?? []).map((s) => ({
      name: s.name,
      command: s.command,
      status: s.status,
      output: s.output,
      error: s.error,
      warning: s.warning,
      warningDetail: s.warning_detail,
      startedAt: s.started_at,
      endedAt: s.ended_at,
    })),
    errorMessage: pr.error_message,
    durationMs: pr.duration_ms,
  };
}
