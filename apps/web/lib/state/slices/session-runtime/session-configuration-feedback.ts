import type { SliceSet } from "@/lib/state/slice-creator";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import type { Turn } from "@/lib/types/http";
import type { SessionRuntimeSlice } from "./types";

export type ConfigurationFeedbackOutcome =
  | { kind: "changed"; label: string; value: string }
  | { kind: "saved"; label: string; value: string }
  | { kind: "unconfirmed"; label: string; value: string; reason: string }
  | { kind: "notice"; text: string; dismissible: boolean };

export type ConfigurationFeedbackEntry = ConfigurationFeedbackOutcome & { configId: string };

type ConfigurationFeedbackRetry = {
  run: () => void;
  executionId?: string;
};

export type SessionConfigurationFeedbackRow = {
  id: string;
  turnId: string | null;
  executionId?: string;
  createdAt: string;
  entries: ConfigurationFeedbackEntry[];
  retry?: ConfigurationFeedbackRetry;
};

export type SessionConfigurationFeedbackUpdate = Omit<
  SessionConfigurationFeedbackRow,
  "id" | "retry"
> & { retry?: ConfigurationFeedbackRetry | null };

export type ConfigurationFeedbackState = {
  configurationFeedback: { bySessionId: Record<string, SessionConfigurationFeedbackRow[]> };
};

export type ConfigurationFeedbackActions = {
  recordSessionConfigurationFeedback: (
    sessionId: string,
    update: SessionConfigurationFeedbackUpdate,
  ) => void;
  dismissSessionConfigurationFeedback: (sessionId: string, id: string) => void;
};

export function configurationFeedbackTurnId(turns: Turn[] | undefined): string | null {
  let latestStartedAt: bigint | null = null;
  let latestId: string | null = null;
  for (const turn of turns ?? []) {
    const startedAt = parseTurnTimestamp(turn.started_at);
    if (startedAt !== null && (latestStartedAt === null || startedAt > latestStartedAt)) {
      latestStartedAt = startedAt;
      latestId = turn.id;
    }
  }
  return latestId;
}

export function buildSessionConfigurationFeedbackActions(
  set: SliceSet<SessionRuntimeSlice>,
): ConfigurationFeedbackActions {
  return {
    recordSessionConfigurationFeedback: (
      sessionId: string,
      update: SessionConfigurationFeedbackUpdate,
    ) =>
      set((draft) => {
        const { turnId, executionId, createdAt, entries, retry } = update;
        const rows = draft.configurationFeedback.bySessionId[sessionId] ?? [];
        // Execution identity separates startup settings from live changes before the first turn arrives.
        const row = rows.find((item) => item.turnId === turnId && item.executionId === executionId);
        if (row) {
          if (entries.length > 0) {
            const bySetting = new Map(row.entries.map((entry) => [entry.configId, entry]));
            for (const entry of entries) bySetting.set(entry.configId, entry);
            row.entries = [...bySetting.values()];
          }
          if (retry !== undefined) row.retry = retry ?? undefined;
          return;
        }
        if (entries.length === 0) return;
        draft.configurationFeedback.bySessionId[sessionId] = [
          ...rows,
          {
            id: `configuration-feedback-${sessionId}-${turnId ?? "initial"}-${executionId ?? "saved"}`,
            turnId,
            executionId,
            createdAt,
            entries,
            retry: retry ?? undefined,
          },
        ];
      }),
    dismissSessionConfigurationFeedback: (sessionId: string, id: string) =>
      set((draft) => {
        const rows = draft.configurationFeedback.bySessionId[sessionId];
        if (!rows) return;
        draft.configurationFeedback.bySessionId[sessionId] = rows.filter((row) => row.id !== id);
      }),
  };
}
