import type { Page } from "@playwright/test";

type Session = Record<string, unknown> & { id: string };
type ByTaskValue = { sessions: Session[]; total: number };

type E2EQueryWindow = Window & {
  __KANDEV_E2E_QUERY_CLIENT__?: {
    getQueryData: (key: readonly unknown[]) => unknown;
    setQueryData: (key: readonly unknown[], updater: (prev: unknown) => unknown) => void;
    getQueriesData: (filters: {
      queryKey: readonly unknown[];
    }) => Array<[readonly unknown[], unknown]>;
  };
};

/**
 * Simulate a lean session-list / partial WS update: preserve `is_passthrough`
 * but drop `agent_profile_snapshot` from the client TanStack Query cache.
 *
 * The TaskSession record lives in the TQ caches (`qk.taskSession.byId` +
 * `qk.taskSession.byTask`) since the Zustand mirror was removed. We patch both
 * directly so we bypass `mergeTaskSession`'s nullish-coalescing guard on
 * `agent_profile_snapshot` (see session-slice.ts) — the same thing the old
 * Zustand `setState` bypass did.
 */
export async function stripSessionProfileSnapshot(page: Page, sessionId: string): Promise<void> {
  await page.evaluate((sid) => {
    const qc = (window as E2EQueryWindow).__KANDEV_E2E_QUERY_CLIENT__;
    if (!qc) {
      throw new Error("E2E query client bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
    }

    const byIdKey = ["session", "byId", sid] as const;
    const existing = qc.getQueryData(byIdKey) as Session | null;
    if (!existing) {
      throw new Error(`Session ${sid} not found in the TQ by-id cache`);
    }
    qc.setQueryData(byIdKey, (prev) => ({
      ...(prev as Session),
      agent_profile_snapshot: undefined,
    }));

    // Mirror the change into every by-task list that contains this session so
    // list-based readers see the lean record too.
    for (const [key, value] of qc.getQueriesData({ queryKey: ["session", "byTask"] })) {
      const list = value as ByTaskValue | undefined;
      if (!list?.sessions.some((s) => s.id === sid)) continue;
      qc.setQueryData(key, (prev) => {
        const cur = prev as ByTaskValue;
        return {
          ...cur,
          sessions: cur.sessions.map((s) =>
            s.id === sid ? { ...s, agent_profile_snapshot: undefined } : s,
          ),
        };
      });
    }

    const updated = qc.getQueryData(byIdKey) as Session | undefined;
    if (updated?.agent_profile_snapshot !== undefined) {
      throw new Error("Failed to strip agent_profile_snapshot from the session cache");
    }
  }, sessionId);
}
