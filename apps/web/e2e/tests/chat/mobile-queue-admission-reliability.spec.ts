import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { registerSeparateQueueRows } from "../../helpers/message-queue-settings";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { routeMainWebSocketWithQueueAdmissionDrops } from "../../helpers/ws-drop";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";
import { SessionPage } from "../../pages/session-page";

registerSeparateQueueRows(test);

async function expectTouchTarget(locator: ReturnType<SessionPage["submitButton"]>): Promise<void> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

async function seedMobileBusyTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ session: SessionPage; taskId: string; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile queue admission request recovery",
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
  await session.sendMessageViaButton("/sleep 30");
  await session.agentStatus().waitFor({ state: "visible", timeout: 20_000 });
  await waitForActiveSessionForegroundActivity(testPage, "generating");
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  return { session, taskId: task.id, sessionId: task.session_id };
}

test.describe("mobile queue admission reliability", () => {
  test.describe.configure({ retries: 1 });

  test("retries a pre-server lost Task admission once and clears the draft", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const drops = await routeMainWebSocketWithQueueAdmissionDrops(testPage);
    const { session, taskId, sessionId } = await seedMobileBusyTask(testPage, apiClient, seedData);
    await waitForComposerQueueMode(session.activeChat());

    const prompt = "recover the lost mobile queue request";
    drops.dropNextQueueAddRequest();
    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
    await typeWhileBusy(testPage, editor, prompt);
    await expectTouchTarget(session.submitButton());
    await session.tapSubmitWhenReady();

    const identity = await apiClient.getQueueSessionIdentity(taskId, sessionId);
    await expect(editor).toHaveText("", { timeout: 45_000 });
    await expect(session.activeChat().getByTestId("queue-chip")).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(async () => (await apiClient.getQueueStatus(identity)).count, { timeout: 15_000 })
      .toBe(1);
    await expect.poll(() => drops.queueAddRequestCount()).toBe(2);
    await expect.poll(() => drops.droppedRequestCount()).toBe(1);
  });

  test("reconciles a lost accepted Quick Chat response through a touch submit", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    const drops = await routeMainWebSocketWithQueueAdmissionDrops(testPage);
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/sleep 30");
    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    await waitForComposerQueueMode(dialog);

    const prompt = "recover the mobile Quick Chat admission";
    drops.dropNextQueueAddResponse();
    const editor = dialog.locator(".tiptap.ProseMirror:visible");
    await typeWhileBusy(testPage, editor, prompt);
    const submit = dialog.getByTestId("submit-message-button");
    await expectTouchTarget(submit);
    await submit.tap();

    await expect(editor).toHaveText("", { timeout: 30_000 });
    await expect(dialog.getByTestId("queue-chip")).toBeVisible({ timeout: 15_000 });
    await expect.poll(() => drops.queueAddRequestCount()).toBe(1);
    await expect.poll(() => drops.droppedResponseCount()).toBe(1);
  });
});
