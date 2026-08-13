import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";
import { expectFullQueueScrolls, seedFullQueueTask } from "./message-queue-scroll-helpers";
import {
  registerSeparateQueueRows,
  requestMessageQueueSettings,
} from "../../helpers/message-queue-settings";

registerSeparateQueueRows(test);

// ---------------------------------------------------------------------------
// Quick Chat queue tests
// ---------------------------------------------------------------------------

/**
 * Open the collapsed queue panel by clicking the floating chip. The chip
 * appears above the chat input once at least one message is queued; the
 * panel only mounts after a click (collapsed by default).
 */
async function openQueuePanel(scope: Locator | Page): Promise<void> {
  const chip = scope.getByTestId("queue-chip");
  await expect(chip).toBeVisible({ timeout: 10_000 });
  await chip.click();
  await expect(scope.getByTestId("queued-ghost-list")).toBeVisible({ timeout: 5_000 });
}

async function openQuickChatWithAgent(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");

  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+q`);

  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  const setup = dialog.getByTestId("quick-chat-setup");
  if (!(await setup.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await page.getByTestId("quick-chat-new-agent").click();
  }
  await expect(setup).toBeVisible({ timeout: 5_000 });

  const agentSelector = dialog.getByTestId("agent-profile-selector");
  if (
    await agentSelector
      .getByText("Select agent", { exact: false })
      .isVisible()
      .catch(() => false)
  ) {
    await agentSelector.click();
    await page.getByRole("option").first().click();
  }
  await dialog.getByTestId("quick-chat-start").click();

  // Wait for chat input to appear AND become editable. Eager init means the
  // agent starts during the picker → tab transition; the input is briefly
  // disabled while the FE store catches up to the RUNNING session state.
  //
  // Race fix: `contenteditable="true"` was observed as a momentary flicker
  // before the session settled into STARTING/RUNNING and flipped the input
  // back to false. Callers then hit `editor.fill()` against a non-editable
  // node and the test failed. Wait for the agent-status indicator to clear
  // (STARTING/RUNNING both render a "Agent is …" status; IDLE renders none),
  // then assert editability — by that point the input has reached its
  // stable, ready state.
  const editor = dialog.locator(".tiptap.ProseMirror");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("status", { name: /Agent is (starting|running)/ })).not.toBeVisible({
    timeout: 30_000,
  });
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
  return dialog;
}

test.describe("Quick chat queue", () => {
  // Allow 1 retry: the test can be flaky when a previous test cycle's agent process hasn't
  // fully shut down, causing the new session to conflict with a stale execution.
  test.describe.configure({ retries: 1 });

  test("queued message indicator appears and message executes after agent turn", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    const dialog = await openQuickChatWithAgent(testPage);

    // Send a slow command so the agent stays busy for 10 seconds.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await typeWhileBusy(testPage, editor, "/slow 10s");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    // Wait for agent to become busy.
    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    await waitForComposerQueueMode(dialog);

    await typeWhileBusy(testPage, editor, "hello world");
    await testPage.keyboard.press(`${modifier}+Enter`);

    // Collapsed-by-default chip is the new queued-message indicator.
    const chip = dialog.getByTestId("queue-chip");
    await expect(chip).toBeVisible({ timeout: 10_000 });
    // The chip is only rendered while the queue panel is collapsed, so its
    // mere presence implies the closed state — no data-open assertion needed.

    // Wait for the first (slow) response to complete.
    await expect(dialog.getByText("Slow response complete", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // The queued message should auto-execute — wait for the agent turn to finish.
    await expect(
      dialog.locator('[data-placeholder="Continue working on the task..."]'),
    ).toBeVisible({
      timeout: 30_000,
    });
  });

  test("queue message via submit button click", async ({ testPage }) => {
    test.setTimeout(90_000);

    const dialog = await openQuickChatWithAgent(testPage);

    // Send a slow command so the agent stays busy for 10 seconds.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await typeWhileBusy(testPage, editor, "/slow 10s");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    // Wait for agent to become busy.
    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    await waitForComposerQueueMode(dialog);

    // Before typing, only the cancel button should be visible (no send button).
    const submitBtn = dialog.getByTestId("submit-message-button");
    await expect(submitBtn).not.toBeVisible();
    await expect(dialog.getByTestId("cancel-agent-button")).toBeVisible();

    // Type a queued message — the submit button should appear.
    await typeWhileBusy(testPage, editor, "queued via button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });

    // Click the submit button (not keyboard shortcut) to queue the message.
    await submitBtn.click();

    // Verify the collapsed chip appears as the queued-message indicator.
    await expect(dialog.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });

    // Verify the cancel-agent button is also visible alongside submit.
    const cancelAgentBtn = dialog.getByTestId("cancel-agent-button");
    await expect(cancelAgentBtn).toBeVisible();

    // Wait for the first (slow) response to complete and queued message to auto-execute.
    await expect(
      dialog.locator('[data-placeholder="Continue working on the task..."]'),
    ).toBeVisible({
      timeout: 60_000,
    });
  });
});

// ---------------------------------------------------------------------------
// Task session queue tests
// ---------------------------------------------------------------------------

async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description = "/e2e:simple-message",
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  return session;
}

async function queueMessagesWhileBusy(
  page: Page,
  editor: Locator,
  messages: string[],
): Promise<void> {
  const submit = page.getByTestId("submit-message-button");
  for (const message of messages) {
    await typeWhileBusy(page, editor, message);
    await expect(submit).toBeVisible({ timeout: 5_000 });
    await submit.click();
  }
}

test.describe("Task session queue", () => {
  test.describe.configure({ retries: 1 });

  test("full queue scrolls internally without hiding the composer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedFullQueueTask(
      testPage,
      apiClient,
      seedData,
      "Desktop full queue scrolling",
    );

    await expectFullQueueScrolls(session);
  });

  test("automatic merge compacts compatible user rows and disabled mode keeps them separate", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Automatic queue merge", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
      agentProfileId: seedData.agentProfileId,
    });
    await requestMessageQueueSettings(apiClient, "PATCH", { auto_merge_enabled: true });

    await apiClient.queueMessage(task.id, sessionId, "automatic first");
    await apiClient.queueMessage(task.id, sessionId, "automatic second");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await openQueuePanel(session.activeChat());
    const panel = session.activeChat().getByTestId("queued-ghost-list");
    const entries = panel.getByTestId("queue-entry-text");
    await expect(entries).toHaveCount(1);
    await expect(entries.first()).toContainText("automatic first");
    await expect(entries.first()).toContainText("automatic second");

    await requestMessageQueueSettings(apiClient, "PATCH", { auto_merge_enabled: false });
    await apiClient.queueMessage(task.id, sessionId, "separate third");
    await apiClient.queueMessage(task.id, sessionId, "separate fourth");
    await expect(entries).toHaveCount(3, { timeout: 10_000 });
    await expect(entries.nth(1)).toHaveText("separate third");
    await expect(entries.nth(2)).toHaveText("separate fourth");
  });

  test("queue message via submit button on task session page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue button test",
    );

    // Send a slow command to keep the agent busy.
    await session.sendMessage("/slow 5s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForComposerQueueMode(testPage);

    // Type a message while agent is busy.
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "queued via button");

    // Both submit and cancel-agent buttons should be visible.
    const submitBtn = testPage.getByTestId("submit-message-button");
    const cancelAgentBtn = testPage.getByTestId("cancel-agent-button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });
    await expect(cancelAgentBtn).toBeVisible();

    // Click the submit button to queue the message.
    await submitBtn.click();

    // Collapsed chip is the queued-message indicator on the task session page.
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });

    // Expand once so we can verify the per-entry Remove control is present.
    await openQueuePanel(testPage);
    await expect(testPage.getByTitle("Remove queued message")).toBeVisible({ timeout: 5_000 });

    // Wait for the queued message to auto-execute and agent to become idle.
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("row Send Now replaces the active turn and preserves neighboring entries", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue Send Now row test",
    );
    await session.sendMessage("/slow 30s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(session.chat.getByText("Running slow response (30s total)...")).toBeVisible({
      timeout: 15_000,
    });
    const taskID = new URL(testPage.url()).pathname.split("/").pop();
    if (!taskID) throw new Error("task URL did not contain a task ID");
    const workflowStepBefore = (await apiClient.getTask(taskID)).workflow_step_id;

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await queueMessagesWhileBusy(testPage, editor, [
      "send first now",
      "/slow 10s second now",
      "send third now",
    ]);

    await openQueuePanel(testPage);
    const panel = testPage.getByTestId("queued-ghost-list");
    await expect(panel.getByTestId("queue-entry-text")).toHaveCount(3);
    const target = panel.getByTestId("queue-entry").filter({ hasText: "second now" });
    await expect(target).toBeVisible();
    await target.hover();
    const sendNow = target.getByTestId("queue-entry-send-now");
    await expect(sendNow).toBeVisible({ timeout: 10_000 });
    await expect(sendNow).toBeEnabled({ timeout: 10_000 });
    await sendNow.click();

    const transcript = session.chat.locator(".chat-message-list:visible");
    await expect(transcript.getByText("/slow 10s second now", { exact: true })).toBeVisible({
      timeout: 20_000,
    });
    await expect(panel.getByTestId("queue-entry-text")).toHaveCount(2, { timeout: 10_000 });
    await expect(panel.getByTestId("queue-entry-text").nth(0)).toContainText("send first now");
    await expect(panel.getByTestId("queue-entry-text").nth(1)).toContainText("send third now");
    await expect(session.chat).not.toContainText("Turn cancelled by user");
    await expect
      .poll(async () => (await apiClient.getTask(taskID)).workflow_step_id, { timeout: 10_000 })
      .toBe(workflowStepBefore);
  });

  test("header Send Now replaces the turn with the visible queue in FIFO order", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue Send Now bulk test",
    );
    await session.sendMessage("/slow 30s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForComposerQueueMode(testPage);

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await queueMessagesWhileBusy(testPage, editor, ["bulk first", "bulk second", "bulk third"]);
    await openQueuePanel(testPage);
    const panel = testPage.getByTestId("queued-ghost-list");
    await expect(panel.getByTestId("queue-entry-text")).toHaveCount(3);

    await panel.getByTestId("queue-send-now").click();
    await expect(panel).not.toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible({ timeout: 15_000 });

    const transcript = session.chat.locator(".chat-message-list:visible");
    await expect(transcript).toContainText("bulk first", { timeout: 20_000 });
    await expect(transcript).toContainText("bulk second");
    await expect(transcript).toContainText("bulk third");
    const aggregateUserBubble = transcript
      .getByTestId("user-message-bubble")
      .filter({ hasText: "bulk first" });
    await expect(aggregateUserBubble).toHaveCount(1);
    await expect(aggregateUserBubble).toContainText("bulk second");
    await expect(aggregateUserBubble).toContainText("bulk third");
    const transcriptText = await transcript.innerText();
    expect(transcriptText.indexOf("bulk first")).toBeLessThan(
      transcriptText.indexOf("bulk second"),
    );
    expect(transcriptText.indexOf("bulk second")).toBeLessThan(
      transcriptText.indexOf("bulk third"),
    );
    await session.waitForChatIdle({ timeout: 45_000 });
  });

  test("queue editor textarea scrolls when content is long", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue editor scroll test",
    );

    // Send a slow command to keep the agent busy.
    await session.sendMessage("/slow 10s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForComposerQueueMode(testPage);

    // Type a short message while agent is busy and queue it.
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "short queued msg");

    const submitBtn = testPage.getByTestId("submit-message-button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });
    await submitBtn.click();

    // Expand the collapsed queue panel to reveal the per-entry Edit affordance.
    await openQueuePanel(testPage);

    const editBtn = testPage.getByTitle("Edit queued message");
    await expect(editBtn).toBeVisible({ timeout: 10_000 });
    await editBtn.click();

    // The edit textarea should now be visible.
    const textarea = testPage.getByTestId("queue-edit-textarea");
    await expect(textarea).toBeVisible({ timeout: 5_000 });

    // Fill via native setter + React event so the controlled component updates.
    const longText = Array.from(
      { length: 30 },
      (_, i) => `Line ${i + 1} of scroll test content`,
    ).join("\n");
    await textarea.evaluate((el: HTMLTextAreaElement, text: string) => {
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
      setter.call(el, text);
      el.dispatchEvent(new Event("input", { bubbles: true }));
      el.dispatchEvent(new Event("change", { bubbles: true }));
    }, longText);

    // Verify the textarea has a constrained max-height and is scrollable.
    // The auto-grow effect re-measures after React commits the new value, so
    // poll the metrics rather than sampling them once behind a fixed settle.
    await expect
      .poll(
        async () =>
          textarea.evaluate((el: HTMLTextAreaElement) => ({
            scrollable: el.scrollHeight > el.clientHeight,
            maxHeight: getComputedStyle(el).maxHeight,
            overflowY: getComputedStyle(el).overflowY,
          })),
        { timeout: 5_000 },
      )
      .toEqual({ scrollable: true, maxHeight: "200px", overflowY: "auto" });
  });

  test("merges a queued message into the message above it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(testPage, apiClient, seedData, "Queue merge test");

    // Send a slow command to keep the agent busy while we queue two messages.
    await session.sendMessage("/slow 10s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForComposerQueueMode(testPage);

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "first message");
    const submitBtn = testPage.getByTestId("submit-message-button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });
    await submitBtn.click();

    await typeWhileBusy(testPage, editor, "second message");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });
    await submitBtn.click();

    // Expand the queue panel; the second row offers the merge control.
    await openQueuePanel(testPage);
    const mergeButtons = testPage.getByTestId("queue-entry-merge");
    await expect(mergeButtons).toHaveCount(1, { timeout: 5_000 });
    await mergeButtons.click();

    // After the merge the queue holds a single entry whose text contains both
    // messages, and the panel header reflects one queued message.
    const entries = testPage.getByTestId("queue-entry-text");
    await expect(entries).toHaveCount(1, { timeout: 5_000 });
    await expect(entries.first()).toContainText("first message");
    await expect(entries.first()).toContainText("second message");
    await expect(testPage.getByTestId("queued-ghost-list")).toContainText(/1 of 10/);
  });

  test("queue message with plan mode enabled via submit button", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue plan mode test",
    );

    // Enable plan mode.
    await session.togglePlanMode();

    // Send a slow command to keep the agent busy.
    await session.sendMessage("/slow 5s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForComposerQueueMode(testPage);

    // In plan mode with no typed text, only the cancel button should be visible.
    // The auto-added plan context should NOT cause the send button to appear.
    const submitBtn = testPage.getByTestId("submit-message-button");
    await expect(submitBtn).not.toBeVisible();
    await expect(testPage.getByTestId("cancel-agent-button")).toBeVisible();

    // Type a message while agent is busy — send button should appear.
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "plan queue test");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });

    // Click the submit button to queue the message.
    await submitBtn.click();

    // Expand the chip first; the panel is collapsed by default.
    await openQueuePanel(testPage);

    // Verify the queued ghost list shows clean text (no system tags).
    const queueIndicator = testPage.getByTestId("queued-ghost-list");
    await expect(queueIndicator).toBeVisible({ timeout: 10_000 });
    await expect(queueIndicator).not.toContainText("kandev-system");

    // Wait for agent to finish processing.
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });
  });
});

// ---------------------------------------------------------------------------
// Queue affordance — chip & panel behavior
// ---------------------------------------------------------------------------

test.describe("Queue affordance", () => {
  test.describe.configure({ retries: 1 });

  test("queue chip stays collapsed by default and toggles via panel close button", async ({
    testPage,
  }) => {
    test.setTimeout(90_000);

    const dialog = await openQuickChatWithAgent(testPage);

    // Send a slow command so the agent stays busy long enough to queue.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.fill("/slow 10s");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      { timeout: 15_000 },
    );
    await waitForComposerQueueMode(dialog);

    await typeWhileBusy(testPage, editor, "first queued");
    await testPage.keyboard.press(`${modifier}+Enter`);

    // Chip is present and collapsed by default.
    const chip = dialog.getByTestId("queue-chip");
    await expect(chip).toBeVisible({ timeout: 10_000 });
    await expect(chip).toContainText("queued");
    await expect(dialog.getByTestId("queued-ghost-list")).not.toBeVisible();

    // Clicking the chip expands the panel; the chip itself unmounts because the
    // panel header carries the same affordance.
    await chip.click();
    await expect(dialog.getByTestId("queued-ghost-list")).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByTestId("queue-chip")).not.toBeVisible();

    // The panel header's X button collapses back to the chip.
    await dialog.getByTestId("queue-close").click();
    await expect(dialog.getByTestId("queued-ghost-list")).not.toBeVisible();
    await expect(dialog.getByTestId("queue-chip")).toBeVisible({ timeout: 5_000 });
  });

  test("Escape collapses an open queue panel", async ({ testPage }) => {
    test.setTimeout(90_000);

    const dialog = await openQuickChatWithAgent(testPage);

    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.fill("/slow 10s");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      { timeout: 15_000 },
    );
    await waitForComposerQueueMode(dialog);

    await typeWhileBusy(testPage, editor, "queued for esc");
    await testPage.keyboard.press(`${modifier}+Enter`);

    await openQueuePanel(dialog);
    // Move focus out of the editor so Escape isn't swallowed by the textarea guard.
    await dialog.getByTestId("queue-close").focus();
    await testPage.keyboard.press("Escape");

    await expect(dialog.getByTestId("queued-ghost-list")).not.toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByTestId("queue-chip")).toBeVisible({ timeout: 5_000 });
  });

  test("clear-all from the panel empties the queue and hides the chip", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Queue clear-all test",
    );

    await session.sendMessage("/slow 10s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForComposerQueueMode(testPage);

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "to be cleared");
    const submitBtn = testPage.getByTestId("submit-message-button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });
    await submitBtn.click();

    await openQueuePanel(testPage);
    await testPage.getByTestId("queue-clear-all").click();

    // Panel and chip both disappear once the queue is empty.
    await expect(testPage.getByTestId("queued-ghost-list")).not.toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible({ timeout: 5_000 });
  });
});
