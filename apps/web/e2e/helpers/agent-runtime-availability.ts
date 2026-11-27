import type { Page } from "@playwright/test";
import type { AgentRuntimeAvailability } from "@/lib/types/agent-runtime";

export type AgentRuntimeAvailabilitySnapshot = AgentRuntimeAvailability;

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      setAgentRuntime: (snapshot: AgentRuntimeAvailabilitySnapshot | null) => void;
      agentRuntime: AgentRuntimeAvailabilitySnapshot | null;
    };
    subscribe?: (
      listener: (state: { agentRuntime: AgentRuntimeAvailabilitySnapshot | null }) => void,
    ) => () => void;
  };
  __KANDEV_E2E_AGENT_RUNTIME_HISTORY__?: Array<AgentRuntimeAvailabilitySnapshot | null>;
};

export async function setAgentRuntimeAvailability(
  page: Page,
  snapshot: AgentRuntimeAvailabilitySnapshot | null,
): Promise<void> {
  await page.evaluate((nextSnapshot) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    store.getState().setAgentRuntime(nextSnapshot);
  }, snapshot);
}

export async function observeAgentRuntimeAvailability(page: Page): Promise<void> {
  await page.evaluate(() => {
    const win = window as E2EStoreWindow;
    const store = win.__KANDEV_E2E_STORE__;
    if (!store?.subscribe) throw new Error("E2E store subscription missing");
    const history = (win.__KANDEV_E2E_AGENT_RUNTIME_HISTORY__ = [store.getState().agentRuntime]);
    store.subscribe((state) => history.push(state.agentRuntime));
  });
}

export async function waitForAgentRuntimeReplacement(
  page: Page,
  previousEpoch: number,
  bootID: string,
): Promise<void> {
  await page.waitForFunction(
    ({ minimumEpoch, expectedBootID }) => {
      const win = window as E2EStoreWindow;
      const history = win.__KANDEV_E2E_AGENT_RUNTIME_HISTORY__ ?? [];
      return (
        history.some((snapshot) => snapshot?.status === "recovering") &&
        history.some(
          (snapshot) =>
            snapshot?.status === "available" &&
            snapshot.boot_id === expectedBootID &&
            (snapshot.runtime_epoch ?? 0) > minimumEpoch,
        )
      );
    },
    { minimumEpoch: previousEpoch, expectedBootID: bootID },
  );
}

export async function stubAgentRuntimeRestart(page: Page): Promise<() => number> {
  let restartRequests = 0;

  await page.route("**/api/v1/system/restart-capability", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ supported: true, mode: "supervisor", adapter: "supervisor" }),
    }),
  );
  await page.route("**/api/v1/system/info", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ boot_id: "e2e-stable-boot" }),
    }),
  );
  await page.route("**/api/v1/system/restart", (route) => {
    restartRequests += 1;
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ accepted: true, message: "accepted" }),
    });
  });

  return () => restartRequests;
}
