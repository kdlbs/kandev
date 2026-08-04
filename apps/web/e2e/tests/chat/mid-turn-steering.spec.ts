import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

// End-to-end coverage for the mid-turn steering composer contract
// (docs/specs/platform/mid-turn-steering.md). CONTRIBUTING.md requires Playwright
// coverage for UI changes; this exercises the flag-gated decision and the real
// steer dispatch path through the mock agent, which advertises promptQueueing.
//
// The observable contract is delivered-vs-queued, not the placeholder text: the
// tiptap placeholder is a cursor-anchored decoration and only renders under
// focus, so asserting message delivery is the robust signal.

async function seedRunningGeneratingSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  // A no-tool sleep holds the foreground turn open and generating — the exact
  // state that normally queues input, so it isolates the steering gate.
  await session.sendMessage("/sleep 20");
  await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
  await waitForActiveSessionForegroundActivity(testPage, "generating");
  return session;
}

test.describe.serial("Claude mid-turn steering experiment", () => {
  test.describe.configure({ retries: 1 });

  test.describe("enabled", () => {
    test.beforeAll(async ({ backend }) => {
      await backend.restart({ KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING: "true" });
      const response = await fetch(`${backend.baseUrl}/api/v1/features`);
      expect(response.ok).toBeTruthy();
      expect(await response.json()).toMatchObject({ claudeMidTurnSteering: true });
    });

    test.afterAll(async ({ backend }) => {
      await backend.restart();
    });

    test("delivers a message into a generating turn instead of queuing it", async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      test.setTimeout(120_000);
      const session = await seedRunningGeneratingSession(
        testPage,
        apiClient,
        seedData,
        "Mid-turn steering enabled",
      );

      // The session reports steer capability: the composer will deliver rather
      // than queue.
      await waitForActiveSessionSupportsSteering(testPage, true);

      const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
      await typeWhileBusy(testPage, editor, "steer: change course now");
      await testPage.getByTestId("submit-message-button").click();

      // A steered send is delivered, not queued: it appears as a user message
      // and must NOT produce a client-side queue chip.
      await expect(session.activeChat().getByText("steer: change course now")).toBeVisible({
        timeout: 15_000,
      });
      await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
    });
  });

  test.describe("disabled by default", () => {
    test("queues input for a generating turn when the flag is off", async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      test.setTimeout(120_000);
      const session = await seedRunningGeneratingSession(
        testPage,
        apiClient,
        seedData,
        "Mid-turn steering disabled",
      );

      // Default profile: the flag is off, so a generating session keeps today's
      // queue behavior even though the mock agent advertises prompt queueing.
      await waitForActiveSessionSupportsSteering(testPage, false);

      const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
      await typeWhileBusy(testPage, editor, "queue this follow-up");
      await testPage.getByTestId("submit-message-button").click();
      await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
    });
  });
});

async function waitForActiveSessionSupportsSteering(page: Page, expected: boolean): Promise<void> {
  await expect
    .poll(
      async () =>
        page.evaluate(() => {
          const store = (
            window as Window & {
              __KANDEV_E2E_STORE__?: {
                getState: () => {
                  tasks: { activeSessionId: string | null };
                  taskSessions: { items: Record<string, { supports_steering?: boolean }> };
                };
              };
            }
          ).__KANDEV_E2E_STORE__;
          // Report undefined — never false — until the session and its
          // supports_steering field actually exist. Collapsing "not loaded yet"
          // into false would let the flag-off assertion pass before the
          // capability arrives, or when a regression drops the field entirely.
          if (!store) return undefined;
          const state = store.getState();
          const sid = state.tasks.activeSessionId;
          if (!sid) return undefined;
          return state.taskSessions.items[sid]?.supports_steering;
        }),
      { timeout: 15_000 },
    )
    .toBe(expected);
}
