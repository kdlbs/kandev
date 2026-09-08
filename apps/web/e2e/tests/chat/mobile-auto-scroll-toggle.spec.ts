import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { dwell } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { waitForStableActiveSession } from "../../helpers/session-store";
import { routeMainWebSocketWithMessageListResponseHold } from "../../helpers/ws-response-hold";
import { watchOlderMessageRequests } from "./message-pagination-helpers";

const MOBILE_END_TOLERANCE_PX = 10;

function mobileOverflowScript(prefix: string): string {
  return Array.from(
    { length: 30 },
    (_, index) =>
      `e2e:message("${prefix} ${index + 1} - lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua")`,
  ).join("\n");
}

async function switchMobileTask(testPage: Page, title: string): Promise<void> {
  await testPage.getByTestId("mobile-session-menu").tap();
  const sheet = testPage.getByRole("dialog", { name: "Tasks" });
  const taskRow = sheet.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(taskRow).toBeVisible({ timeout: 15_000 });
  // This test covers task-switch transcript continuity, not touch activation.
  // Use a click so the row's long-press drag sensor cannot consume the gesture
  // while the mobile drawer is under CI load.
  await taskRow.click();
  await expect(sheet).not.toBeVisible({ timeout: 10_000 });
}

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

  test("task switch places cached history before its refresh settles", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile cached transcript A",
      seedData.agentProfileId,
      {
        description: mobileOverflowScript("Mobile environment A history"),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile cached transcript B",
      seedData.agentProfileId,
      {
        description: mobileOverflowScript("Mobile environment B history"),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!taskA.session_id || !taskB.session_id) {
      throw new Error("createTaskWithAgent did not return both session ids");
    }
    await waitForSessionDone(apiClient, taskA.id, taskA.session_id, "mobile task A should finish");
    await waitForSessionDone(apiClient, taskB.id, taskB.session_id, "mobile task B should finish");

    const refreshHold = await routeMainWebSocketWithMessageListResponseHold(testPage);
    const olderRequests = watchOlderMessageRequests(testPage, taskA.session_id);
    await testPage.goto(`/t/${taskA.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await waitForStableActiveSession(testPage, taskA.session_id);
    const list = session.activeChat().locator(".chat-message-list");
    await expect
      .poll(async () => list.evaluate((element) => element.scrollHeight - element.clientHeight), {
        timeout: 15_000,
        message: "mobile task A transcript should overflow",
      })
      .toBeGreaterThan(200);

    await switchMobileTask(testPage, "Mobile cached transcript B");
    await waitForStableActiveSession(testPage, taskB.session_id);
    refreshHold.holdNextLatestWindow(taskA.session_id);
    await switchMobileTask(testPage, "Mobile cached transcript A");
    await waitForStableActiveSession(testPage, taskA.session_id);
    await expect.poll(refreshHold.heldCount, { timeout: 5_000 }).toBe(1);
    await expect
      .poll(
        async () =>
          list.evaluate(
            (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
          ),
        {
          timeout: 2_000,
          message: "mobile cached transcript should be at the bottom before refresh release",
        },
      )
      .toBeLessThan(MOBILE_END_TOLERANCE_PX);
    refreshHold.releaseHeldResponse();
    await expect
      .poll(async () =>
        list.evaluate((element) => element.scrollHeight - element.scrollTop - element.clientHeight),
      )
      .toBeLessThan(MOBILE_END_TOLERANCE_PX);
    await dwell(
      testPage,
      500,
      "negative-assertion",
      "observe mobile pagination after first refresh release",
    );
    expect(olderRequests).toHaveLength(0);

    const targetScrollTop = await list.evaluate((element) => {
      element.scrollTop = Math.floor((element.scrollHeight - element.clientHeight) / 2);
      element.dispatchEvent(new Event("scroll"));
      return element.scrollTop;
    });
    const toggle = session.chatStatusBar().getByTestId("auto-scroll-toggle-button");
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");

    await switchMobileTask(testPage, "Mobile cached transcript B");
    await waitForStableActiveSession(testPage, taskB.session_id);
    refreshHold.holdNextLatestWindow(taskA.session_id);
    await switchMobileTask(testPage, "Mobile cached transcript A");
    await waitForStableActiveSession(testPage, taskA.session_id);
    await expect.poll(refreshHold.heldCount, { timeout: 5_000 }).toBe(1);
    await expect
      .poll(
        async () =>
          Math.abs((await list.evaluate((element) => element.scrollTop)) - targetScrollTop),
        {
          timeout: 2_000,
          message:
            "mobile cached transcript should restore its saved position before refresh release",
        },
      )
      .toBeLessThanOrEqual(20);
    refreshHold.releaseHeldResponse();
    await expect
      .poll(async () =>
        Math.abs((await list.evaluate((element) => element.scrollTop)) - targetScrollTop),
      )
      .toBeLessThanOrEqual(20);
    await dwell(
      testPage,
      500,
      "negative-assertion",
      "observe mobile pagination after second refresh release",
    );
    expect(olderRequests).toHaveLength(0);

    const documentWidth = await testPage.evaluate(() => ({
      clientWidth: document.documentElement.clientWidth,
      scrollWidth: document.documentElement.scrollWidth,
    }));
    expect(documentWidth.scrollWidth).toBeLessThanOrEqual(documentWidth.clientWidth);
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
