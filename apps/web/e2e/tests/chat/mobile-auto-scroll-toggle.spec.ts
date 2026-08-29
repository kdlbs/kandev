import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

/**
 * Seed a task whose mock-agent script emits enough distinct messages to
 * overflow the chat list, then open it and wait for the turn to finish.
 * Mirrors `auto-scroll-toggle.spec.ts`'s desktop fixture — kept as a
 * separate local copy rather than a shared import so this mobile-only file
 * stays self-contained (mobile specs live in their own Playwright project).
 */
async function seedOverflowingTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  messageCount = 30,
): Promise<SessionPage> {
  const script = Array.from(
    { length: messageCount },
    (_, i) =>
      `e2e:message("Filler message ${i + 1} - lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua")`,
  ).join("\n");

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: script,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "mobile overflow seed session should finish before opening the transcript",
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Mobile transcript auto-scroll toggle", () => {
  test.describe.configure({ timeout: 120_000 });

  test("can be hidden without changing the mobile transcript auto-scroll default", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ show_transcript_auto_scroll_control: false });
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Mobile Auto-scroll Toggle Hidden",
    );
    const list = session.activeChat().locator(".chat-message-list");
    await expect
      .poll(async () => list.evaluate((el) => el.scrollHeight - el.clientHeight), {
        timeout: 15_000,
        message: "Waiting for chat to overflow",
      })
      .toBeGreaterThan(200);

    await expect(session.chatStatusBar().getByTestId("auto-scroll-toggle-button")).toHaveCount(0);
    await expect
      .poll(async () => list.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight))
      .toBeLessThan(25);
  });

  test("is reachable and toggles by touch", async ({ testPage, apiClient, seedData }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Mobile Auto-scroll Toggle Reachable",
      2,
    );

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await expect(toggle).toBeVisible({ timeout: 10_000 });
    await expect(toggle).toBeInViewport();
    await expect(toggle).toBeEnabled();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");

    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
  });

  test("disabling freezes the position and suppresses auto-scroll for new messages in the mobile chat layout", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Mobile Auto-scroll Toggle Freeze",
    );

    const activeChat = session.activeChat();
    const list = activeChat.locator(".chat-message-list");
    await expect(list).toHaveCount(1);
    await expect
      .poll(async () => list.evaluate((el) => el.scrollHeight - el.clientHeight), {
        timeout: 15_000,
        message: "Waiting for chat to overflow",
      })
      .toBeGreaterThan(200);

    const targetScrollTop = await list.evaluate((el) => {
      el.scrollTop = Math.floor((el.scrollHeight - el.clientHeight) / 2);
      return el.scrollTop;
    });
    expect(targetScrollTop).toBeGreaterThan(100);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    // A brand-new message arrives while scrolled away from the bottom and
    // disabled — the view must not jump.
    const marker = "New content while disabled on mobile";
    await session.sendMessageViaButton(`e2e:message("${marker}")`);
    await expect(activeChat.getByText(marker, { exact: false }).last()).toBeVisible({
      timeout: 15_000,
    });
    await expect
      .poll(async () => list.evaluate((el) => el.scrollTop), { timeout: 2_000 })
      .toBeLessThan(targetScrollTop + 10);
    expect(await list.evaluate((el) => el.scrollTop)).toBeGreaterThan(targetScrollTop - 10);
  });

  test("enabled auto-scroll stays at the bottom for live messages on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Mobile Auto-scroll Toggle Enabled Live Message",
    );
    const activeChat = session.activeChat();
    const list = activeChat.locator(".chat-message-list");
    await expect
      .poll(async () => list.evaluate((el) => el.scrollHeight - el.clientHeight), {
        timeout: 15_000,
        message: "Waiting for chat to overflow",
      })
      .toBeGreaterThan(200);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await expect(toggle).toHaveAttribute("aria-pressed", "true");

    const marker = "New content while enabled on mobile";
    await session.sendMessageViaButton(`e2e:message("${marker}")`);
    await expect(activeChat.getByText(marker, { exact: false }).last()).toBeVisible({
      timeout: 15_000,
    });
    await expect
      .poll(async () => list.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight), {
        timeout: 5_000,
        message: "enabled mobile auto-scroll should stay at the bottom after live content",
      })
      .toBeLessThan(10);
  });

  test("disabling from the bottom freezes the view when new content arrives", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedOverflowingTask(
      testPage,
      apiClient,
      seedData,
      "Mobile Auto-scroll Toggle Bottom Anchor",
    );
    const activeChat = session.activeChat();
    const list = activeChat.locator(".chat-message-list");
    // Establish the true-bottom precondition after the mobile sticky prompt
    // bar has joined the scroll layout.
    await expect
      .poll(
        async () =>
          list.evaluate((el) => {
            el.scrollTop = el.scrollHeight;
            return el.scrollHeight - el.scrollTop - el.clientHeight;
          }),
        {
          timeout: 5_000,
          message: "expected to be at the bottom before disabling",
        },
      )
      .toBeLessThan(5);

    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    const marker = "New content while disabled at bottom on mobile";
    await session.sendMessageViaButton(`e2e:delay(500)\ne2e:message("${marker}")`);
    // Mobile submission clears the composer and appends the user's prompt,
    // which can resize the transcript before the delayed agent reply. Capture
    // the frozen position after that submit layout settles so this assertion
    // isolates movement caused by the incoming content.
    const frozenScrollTop = await list.evaluate((el) => el.scrollTop);
    await expect(activeChat.getByText(marker, { exact: false }).last()).toBeVisible({
      timeout: 15_000,
    });

    await expect
      .poll(
        async () =>
          list.evaluate(
            (el, expectedScrollTop) => Math.abs(el.scrollTop - expectedScrollTop),
            frozenScrollTop,
          ),
        { timeout: 2_000 },
      )
      .toBeLessThanOrEqual(2);
  });
});
