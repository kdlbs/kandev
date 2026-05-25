import type { Page } from "@playwright/test";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: { items: Record<string, Record<string, unknown>> };
      setTaskSession: (session: Record<string, unknown>) => void;
    };
  };
};

/**
 * Simulate a lean session-list / partial WS update: keep is_passthrough but drop
 * agent_profile_snapshot from the client store.
 */
export async function stripSessionProfileSnapshot(
  page: Page,
  sessionId: string,
): Promise<void> {
  await page.evaluate((sid) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) {
      throw new Error("E2E store bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
    }
    const session = store.getState().taskSessions.items[sid];
    if (!session) {
      throw new Error(`Session ${sid} not found in store`);
    }
    store.getState().setTaskSession({
      ...session,
      is_passthrough: true,
      agent_profile_snapshot: undefined,
    });
  }, sessionId);
}
