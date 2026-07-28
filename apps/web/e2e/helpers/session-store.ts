import type { Page } from "@playwright/test";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: { items: Record<string, Record<string, unknown>> };
      tasks: { activeSessionId: string | null };
      setAvailableCommands: (sessionId: string, commands: AvailableCommand[]) => void;
    };
    setState: (
      updater: (state: {
        taskSessions: { items: Record<string, Record<string, unknown>> };
      }) => void,
    ) => void;
  };
};

type AvailableCommand = {
  name: string;
  description?: string;
  input_hint?: string;
};

export async function waitForActiveSessionForegroundActivity(
  page: Page,
  activity: "generating" | "background" | null,
): Promise<void> {
  await page.waitForFunction(
    (expected) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) return false;
      const state = store.getState();
      const sessionId = state.tasks.activeSessionId;
      if (!sessionId) return false;
      const current = state.taskSessions.items[sessionId]?.foreground_activity;
      return expected === null ? current == null : current === expected;
    },
    activity,
    { timeout: 20_000 },
  );
}

/**
 * Simulate a lean session-list / partial WS update: preserve `is_passthrough`
 * but drop `agent_profile_snapshot` from the client store.
 *
 * Uses `setState` directly so we bypass `mergeTaskSession`'s nullish-coalescing
 * guard on `agent_profile_snapshot` (see session-slice.ts).
 */
export async function stripSessionProfileSnapshot(page: Page, sessionId: string): Promise<void> {
  await page.evaluate((sid) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) {
      throw new Error("E2E store bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
    }
    store.setState((state) => {
      const session = state.taskSessions.items[sid];
      if (!session) {
        throw new Error(`Session ${sid} not found in store`);
      }
      state.taskSessions.items[sid] = {
        ...session,
        agent_profile_snapshot: undefined,
      };
    });
    const updated = store.getState().taskSessions.items[sid];
    if (updated?.agent_profile_snapshot !== undefined) {
      throw new Error("Failed to strip agent_profile_snapshot from session store");
    }
  }, sessionId);
}

export async function seedAvailableCommands(
  page: Page,
  sessionId: string,
  commands: AvailableCommand[],
): Promise<void> {
  await page.evaluate(
    ({ sid, commandList }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) {
        throw new Error("E2E store bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
      }
      store.getState().setAvailableCommands(sid, commandList);
    },
    { sid: sessionId, commandList: commands },
  );
}
