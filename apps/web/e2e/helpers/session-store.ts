import { expect, type Page } from "@playwright/test";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: { items: Record<string, Record<string, unknown>> };
      tasks: { activeSessionId: string | null };
      quickChat: { activeSessionId: string | null };
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

export async function waitForActiveSessionSupportsSteering(
  page: Page,
  expected = true,
): Promise<void> {
  await page.waitForFunction(
    (expectedValue) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) return false;
      const state = store.getState();
      const sessionId = state.tasks.activeSessionId;
      return sessionId
        ? state.taskSessions.items[sessionId]?.supports_steering === expectedValue
        : false;
    },
    expected,
    {
      timeout: 15_000,
      message: "Active session did not negotiate the expected steering capability",
    },
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
 * Wait until cancellation is backend-owned, or until the fast path has already
 * settled before the projection event can be observed by the browser.
 */
export async function waitForActiveSessionCancellationPendingOrSettled(page: Page): Promise<void> {
  await page.waitForFunction(
    () => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) return false;
      const state = store.getState();
      const sessionId = state.tasks.activeSessionId;
      if (!sessionId) return false;
      const session = state.taskSessions.items[sessionId];
      if (session?.cancellation_pending === true) return true;
      if (session?.foreground_activity !== null && session?.foreground_activity !== undefined) {
        return false;
      }
      return Array.from(document.querySelectorAll<HTMLElement>("[data-placeholder]"))
        .filter((element) => element.offsetWidth > 0 && element.offsetHeight > 0)
        .some((element) => element.dataset.placeholder?.startsWith("Continue working on the "));
    },
    { timeout: 20_000 },
  );
}

export async function waitForActiveQuickChatSupportsSteering(
  page: Page,
  expected = true,
): Promise<string> {
  await page.waitForFunction(
    (expectedValue) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) return false;
      const state = store.getState();
      const sessionId = state.quickChat.activeSessionId;
      return (
        sessionId !== null &&
        state.taskSessions.items[sessionId]?.supports_steering === expectedValue
      );
    },
    expected,
    { timeout: 20_000, message: "Quick Chat did not negotiate the expected steering capability" },
  );
  return page.evaluate(() => {
    const sessionId = (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState().quickChat
      .activeSessionId;
    if (!sessionId) throw new Error("Quick Chat has no active session");
    return sessionId;
  });
}

export async function waitForActiveQuickChatForegroundActivity(
  page: Page,
  activity: "generating" | "background" | null,
): Promise<void> {
  await page.waitForFunction(
    (expected) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) return false;
      const state = store.getState();
      const sessionId = state.quickChat.activeSessionId;
      if (!sessionId) return false;
      const current = state.taskSessions.items[sessionId]?.foreground_activity;
      return expected === null ? current == null : current === expected;
    },
    activity,
    { timeout: 20_000, message: "Quick Chat foreground activity did not reach the expected state" },
  );
}

export async function waitForQuickChatCancellationPending(
  page: Page,
  sessionId: string,
  pending: boolean,
): Promise<void> {
  await page.waitForFunction(
    ({ expected, sid }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      return store?.getState().taskSessions.items[sid]?.cancellation_pending === expected;
    },
    { expected: pending, sid: sessionId },
    { timeout: 20_000, message: `Quick Chat cancellation_pending did not become ${pending}` },
  );
}

export async function waitForQuickChatSessionSettled(page: Page, sessionId: string): Promise<void> {
  await page.waitForFunction(
    (sid) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      const session = store?.getState().taskSessions.items[sid];
      if (!session) return false;
      return (
        ["IDLE", "WAITING_FOR_INPUT", "COMPLETED", "FAILED", "CANCELLED"].includes(
          String(session.state),
        ) &&
        session.cancellation_pending === false &&
        session.foreground_activity == null
      );
    },
    sessionId,
    { timeout: 20_000, message: `Quick Chat session ${sessionId} did not settle` },
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
