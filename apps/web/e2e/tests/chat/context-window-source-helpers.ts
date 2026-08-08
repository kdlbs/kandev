import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

type ContextWindowStore = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      taskSessions: { items: Record<string, { state?: string }> };
      clearContextWindow: (sessionId: string) => void;
      setContextWindow: (sessionId: string, contextWindow: ContextWindowFixture) => void;
    };
    setState: (partial: { clearContextWindow: (sessionId: string) => void }) => void;
  };
};

type ContextWindowFixture = {
  size: number;
  used: number;
  remaining: number;
  efficiency: number;
  compactionCount: number;
  source: "acp" | "api";
};

type ContextWindowSeed = Partial<ContextWindowFixture>;

export async function seedContextWindowTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  seed: ContextWindowSeed = {},
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Context Window Source Test",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Expected the seeded task to start a session");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  // The idle input can render from the terminal text frame before the backend
  // persists the session-state transition. That transition purges runtime
  // context metadata, so injecting the fixture data before it settles can make
  // the context control disappear while the mobile test is tapping it.
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((candidate) => candidate.id === task.session_id)?.state;
      },
      { timeout: 30_000, message: "Waiting for the context session to become idle" },
    )
    .toBe("WAITING_FOR_INPUT");

  // The backend transition can beat this page's WS subscription. Reload from
  // the persisted idle state before injecting runtime-only context data, or a
  // late hydration can purge the fixture while a touch interaction is in
  // progress.
  await testPage.reload();
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await expect
    .poll(() =>
      testPage.evaluate(
        (sessionId) =>
          (window as ContextWindowStore).__KANDEV_E2E_STORE__?.getState().taskSessions.items[
            sessionId
          ]?.state,
        task.session_id!,
      ),
    )
    .toBe("WAITING_FOR_INPUT");

  // Context metadata is runtime-only in this fixture. Protect the synthetic
  // value from duplicate late session/model hydration frames that carry an
  // explicit empty context window; those frames are unrelated to the touch UI
  // contract under test and otherwise detach the trigger mid-tap.
  await testPage.evaluate((sessionId) => {
    const store = (window as ContextWindowStore).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    const clearContextWindow = store.getState().clearContextWindow;
    store.setState({
      clearContextWindow: (candidateId) => {
        if (candidateId !== sessionId) clearContextWindow(candidateId);
      },
    });
  }, task.session_id!);

  const contextWindowFixture: ContextWindowFixture = {
    size: 258_400,
    used: 54_100,
    remaining: 204_300,
    efficiency: 21,
    compactionCount: 2,
    source: "acp",
    ...seed,
  };
  await testPage.evaluate(
    ({ sessionId, fixture }) => {
      const store = (window as ContextWindowStore).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.getState().setContextWindow(sessionId, fixture);
    },
    { sessionId: task.session_id!, fixture: contextWindowFixture },
  );

  const triggerName =
    contextWindowFixture.used === 0
      ? "Context window: usage not measured"
      : `Context window: ${((contextWindowFixture.used / contextWindowFixture.size) * 100).toFixed(
          0,
        )}% used`;
  await expect(testPage.getByRole("button", { name: triggerName })).toBeVisible();
}

export async function expectCompactionCount(contextTooltip: Locator): Promise<void> {
  const contextUsage = contextTooltip.getByTestId("context-window-usage").first();
  const row = contextUsage.getByTestId("context-window-compactions-row");
  await expect(row.getByText("Compactions", { exact: true })).toBeVisible();
  await expect(row.getByText("2", { exact: true })).toBeVisible();
}

export async function expectSourceRightOfTokenCount(contextTooltip: Locator): Promise<void> {
  const row = contextTooltip.getByTestId("context-window-token-row").first();
  await expect(row).toBeVisible();
  const positions = await row.evaluate((element) => {
    const tokenCount = element.firstElementChild;
    const source = element.lastElementChild;
    if (!(tokenCount instanceof HTMLElement) || !(source instanceof HTMLElement)) return null;
    const tokenBox = tokenCount.getBoundingClientRect();
    const sourceBox = source.getBoundingClientRect();
    return { sourceLeft: sourceBox.left, tokenRight: tokenBox.right };
  });

  if (!positions) throw new Error("Expected token count and source to be rendered");
  if (positions.sourceLeft <= positions.tokenRight) {
    throw new Error("Expected context source to render to the right of the token count");
  }
}
