import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import {
  waitForActiveQuickChatForegroundActivity,
  waitForActiveQuickChatSupportsSteering,
  waitForQuickChatCancellationPending,
  waitForQuickChatSessionSettled,
  waitForActiveSessionForegroundActivity,
} from "../../helpers/session-store";
import { seedRunningGeneratingSession } from "../../helpers/generating-session";
import { holdCancellationSettlement } from "../../helpers/cancellation-observation";
import {
  openQuickChatSetup,
  startQuickChatFromSetup,
  sendQuickChatMessage,
  waitForQuickChatDirectInput,
} from "./quick-chat-helpers";

import type { ApiClient } from "../../helpers/api-client";
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";

async function openSettledQuickChat(page: Page, apiClient: ApiClient, navigateHome = true) {
  const dialog = await openQuickChatSetup(page, navigateHome);
  const started = await startQuickChatFromSetup(dialog, page, "/e2e:simple-message");
  await waitForAgentMessage(
    apiClient,
    started.session_id,
    "This is a simple mock response for e2e testing.",
    30_000,
  );
  await waitForSessionDone(
    apiClient,
    started.task_id,
    started.session_id,
    "Quick Chat opening turn must finish before cancellation fixture submission",
    30_000,
  );
  await waitForQuickChatDirectInput(dialog);
  return { dialog, sessionId: started.session_id };
}

function commandDialog(page: Page) {
  return page.locator('[role="dialog"]:has([cmdk-input])').last();
}

test.describe.serial("Quick Chat cancellation palette and composer", () => {
  test.afterEach(async ({ backend }, testInfo) => {
    if (testInfo.status === testInfo.expectedStatus) return;
    await testInfo.attach("quick-chat-cancellation-backend.log", {
      path: backend.logPath,
      contentType: "text/plain",
    });
  });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF: "true",
      KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING: "true",
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("cancels an active Quick Chat turn from its empty composer", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    const { dialog: quickChat, sessionId: quickSessionId } = await openSettledQuickChat(
      testPage,
      apiClient,
    );
    const acceptanceMarker = "Quick Chat composer cancellation accepted";
    await sendQuickChatMessage(quickChat, testPage, `/e2e:cancel-hold ${acceptanceMarker}`);
    await waitForAgentMessage(apiClient, quickSessionId, acceptanceMarker, 30_000);
    expect(await waitForActiveQuickChatSupportsSteering(testPage)).toBe(quickSessionId);
    await waitForActiveQuickChatForegroundActivity(testPage, "generating");
    const editor = quickChat.locator('.tiptap.ProseMirror[contenteditable="true"]:visible');
    await expect(editor).toHaveText("");
    await expect(quickChat.getByTestId("queue-chip")).not.toBeVisible();
    const cancel = quickChat.getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();

    await cancel.click();
    await waitForQuickChatCancellationPending(testPage, quickSessionId, true);
    await expect(cancel).toBeDisabled();
    await waitForQuickChatSessionSettled(testPage, quickSessionId);
    await expect(cancel).not.toBeVisible({ timeout: 15_000 });
  });

  test("cancels Quick Chat while detached background work runs", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    const cancellation = await holdCancellationSettlement(testPage);
    const { dialog: quickChat, sessionId: quickSessionId } = await openSettledQuickChat(
      testPage,
      apiClient,
    );
    await sendQuickChatMessage(quickChat, testPage, "/detached-background 60s");
    await waitForAgentMessage(
      apiClient,
      quickSessionId,
      "Launching detached background work; this foreground turn is complete.",
      30_000,
    );
    await waitForActiveQuickChatForegroundActivity(testPage, "background");
    await expect(
      quickChat.locator('.tiptap.ProseMirror[contenteditable="true"]:visible'),
    ).toHaveText("");
    await expect(quickChat.getByTestId("queue-chip")).not.toBeVisible();
    const cancel = quickChat.getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();

    cancellation.arm(quickSessionId);
    try {
      await cancel.click();
      await waitForQuickChatCancellationPending(testPage, quickSessionId, true);
      await expect(cancel).toBeDisabled();
    } finally {
      cancellation.release();
    }
    await waitForQuickChatSessionSettled(testPage, quickSessionId, 75_000);
    await expect(cancel).not.toBeVisible({ timeout: 15_000 });
  });

  test("cancels Quick Chat and leaves the underlying task turn running", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      "Quick Chat palette cancellation target",
      { predecessorPrompt: "/e2e:cancel-hold Underlying task cancellation fixture accepted" },
    );
    const underlyingCancel = session.activeChat().getByTestId("cancel-agent-button");
    await expect(underlyingCancel).toBeVisible();

    const { dialog: quickChat, sessionId: quickSessionId } = await openSettledQuickChat(
      testPage,
      apiClient,
      false,
    );
    const acceptanceMarker = "Quick Chat palette cancellation accepted";
    await sendQuickChatMessage(quickChat, testPage, `/e2e:cancel-hold ${acceptanceMarker}`);
    await waitForAgentMessage(apiClient, quickSessionId, acceptanceMarker, 30_000);
    await expect(
      quickChat.getByRole("status", { name: /Agent is (starting|running)/ }),
    ).toBeVisible({ timeout: 15_000 });

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const palette = commandDialog(testPage);
    await expect(palette).toBeVisible({ timeout: 10_000 });
    await palette.getByRole("combobox").fill("cancel");
    const cancelOption = palette.getByRole("option").filter({ hasText: /Cancel Turn/i });
    await expect(cancelOption).toHaveCount(1);
    await cancelOption.click();

    await expect(quickChat.getByTestId("cancel-agent-button")).toBeDisabled({ timeout: 5_000 });
    await waitForQuickChatDirectInput(quickChat);
    await waitForActiveSessionForegroundActivity(testPage, "generating");

    await testPage.keyboard.press("Escape");
    await expect(quickChat).not.toBeVisible();
    await expect(underlyingCancel).toBeVisible({ timeout: 10_000 });
    await expect(underlyingCancel).toBeEnabled();
  });
});
