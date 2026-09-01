import { expect, type Page } from "@playwright/test";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: { items: Record<string, Record<string, unknown>> };
      tasks: { activeSessionId: string | null };
      sessionAgentctl: { itemsBySessionId: Record<string, { status?: string }> };
      setAvailableCommands: (sessionId: string, commands: AvailableCommand[]) => void;
      setAuthState: (state: {
        mode: string;
        authenticated: boolean;
        user: StoreUser;
        ssoProviders: unknown[];
      }) => void;
    };
    setState: (
      updater: (state: {
        taskSessions: { items: Record<string, Record<string, unknown>> };
      }) => void,
    ) => void;
  };
};

type StoreUser = {
  id: string;
  email: string;
  display_name: string;
  role: string;
  status: string;
};

type AvailableCommand = {
  name: string;
  description?: string;
  input_hint?: string;
};

/**
 * Inject an authenticated identity through the store bridge.
 *
 * E2E runs with authentication disabled, which leaves the role undefined and
 * every role-gated control in its permissive single-user state. Specs that
 * need to prove a member/admin difference set the identity directly rather
 * than standing up a real login, matching what the system settings specs do.
 */
export async function setStoreRole(
  page: Page,
  role: "member" | "admin",
  overrides: Partial<StoreUser> = {},
): Promise<void> {
  await page.waitForFunction(() => Boolean((window as E2EStoreWindow).__KANDEV_E2E_STORE__));
  await page.evaluate(
    ({ role, overrides }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.getState().setAuthState({
        mode: "enabled",
        authenticated: true,
        user: {
          id: `e2e-${role}`,
          email: `${role}@e2e.dev`,
          display_name: role === "admin" ? "E2E Admin" : "E2E Member",
          role,
          status: "active",
          ...overrides,
        },
        ssoProviders: [],
      });
    },
    { role, overrides },
  );
}

/** Wait until the session agentctl is ready for controls that require it. */
export async function waitForSessionAgentctlReady(
  page: Page,
  sessionId: string,
  timeout = 60_000,
): Promise<void> {
  await page.waitForFunction(
    (sid) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      return store?.getState().sessionAgentctl.itemsBySessionId[sid]?.status === "ready";
    },
    sessionId,
    { timeout, message: `agentctl did not become ready for session ${sessionId}` },
  );
}

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

/** Seed the stale activity projection that can survive a missed reconnect event. */
export async function seedActiveSessionForegroundActivity(
  page: Page,
  activity: "generating" | "background" | null,
): Promise<void> {
  await page.evaluate((nextActivity) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) {
      throw new Error("E2E store bridge missing — is __KANDEV_E2E_EXPOSE_STORE__ set?");
    }
    store.setState((state) => {
      const sessionId = store.getState().tasks.activeSessionId;
      if (!sessionId) throw new Error("No active session is available in the E2E store");
      const session = state.taskSessions.items[sessionId];
      if (!session) throw new Error(`Session ${sessionId} not found in store`);
      // Intentionally skip the activity epoch: this stale projection predates
      // the current backend process and its reconnect events.
      state.taskSessions.items[sessionId] = {
        ...session,
        foreground_activity: nextActivity,
      };
    });
  }, activity);
}

/** Wait for the backend-owned cancellation projection on the active session. */
export async function waitForActiveSessionCancellationPending(
  page: Page,
  pending: boolean,
): Promise<void> {
  await page.waitForFunction(
    (expected) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) return false;
      const sessionId = store.getState().tasks.activeSessionId;
      if (!sessionId) return false;
      return store.getState().taskSessions.items[sessionId]?.cancellation_pending === expected;
    },
    pending,
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

/**
 * Wait until `sessionId` is the active session AND has stopped changing.
 *
 * Clicking a session tab settles asynchronously, and the flicker specs install
 * their observers straight afterwards: starting to observe mid-settle records
 * the tail of the switch as if it were oscillation. Requiring two consecutive
 * agreeing samples gives those specs the quiet baseline the fixed sleeps were
 * approximating, while returning immediately once the switch is genuinely done.
 */
export async function waitForStableActiveSession(
  page: Page,
  sessionId: string,
  timeout = 15_000,
): Promise<void> {
  let previous: string | null = null;
  await expect
    .poll(
      async () => {
        const current = await page.evaluate(
          () => (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState().tasks.activeSessionId,
        );
        const stable = current === sessionId && previous === sessionId;
        previous = current ?? null;
        return stable;
      },
      { timeout, message: `active session did not settle on ${sessionId}` },
    )
    .toBe(true);
}
