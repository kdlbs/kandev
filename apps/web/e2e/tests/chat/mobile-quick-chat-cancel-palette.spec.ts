// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForAgentMessage } from "../../helpers/session";
import { holdCancellationSettlement } from "../../helpers/cancellation-observation";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  waitForActiveQuickChatForegroundActivity,
  waitForActiveQuickChatSupportsSteering,
  waitForQuickChatCancellationPending,
  waitForQuickChatSessionSettled,
} from "../../helpers/session-store";
import {
  sendQuickChatMessage,
  startQuickChatFromSetup,
  waitForQuickChatDirectInput,
  waitForSessionSettledBaseline,
} from "./quick-chat-helpers";

function commandDialog(page: Page) {
  return page.locator('[role="dialog"]:has([cmdk-input])').last();
}

test.use({ trace: "retain-on-failure" });

test.describe.serial("Mobile Quick Chat cancellation", () => {
  test.describe.configure({ retries: 0 });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF: "true",
      KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING: "true",
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("cancels an active Quick Chat turn through the visible touch composer control", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    const cancellation = await holdCancellationSettlement(testPage);
    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const quickChat = testPage.getByRole("dialog", { name: "Quick Chat" });
    const started = await startQuickChatFromSetup(quickChat, testPage);
    await waitForSessionSettledBaseline(apiClient, started.task_id, started.session_id);
    await sendQuickChatMessage(quickChat, testPage, "/slow 30s");
    await waitForAgentMessage(
      apiClient,
      started.session_id,
      "Running slow response (30s total)",
      30_000,
    );

    const quickSessionId = await waitForActiveQuickChatSupportsSteering(testPage);
    expect(quickSessionId).toBe(started.session_id);
    await waitForActiveQuickChatForegroundActivity(testPage, "generating");
    const editor = quickChat.locator('.tiptap.ProseMirror[contenteditable="true"]:visible');
    await expect(editor).toHaveText("");
    await expect(quickChat.getByTestId("queue-chip")).not.toBeVisible();
    const cancel = quickChat.getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();
    expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(44);

    cancellation.arm(quickSessionId);
    try {
      await cancel.tap();
      await waitForQuickChatCancellationPending(testPage, quickSessionId, true);
      await expect(cancel).toBeDisabled();
    } finally {
      cancellation.release();
    }
    await waitForQuickChatSessionSettled(testPage, quickSessionId);
    await expect(cancel).not.toBeVisible({ timeout: 15_000 });
  });

  test("mobile Quick Chat exposes one touch cancellation command", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const cancellation = await holdCancellationSettlement(testPage);
    await testPage.goto("/");
    await testPage.getByTestId("app-nav-trigger").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const quickChat = testPage.getByRole("dialog", { name: "Quick Chat" });
    const { task_id: quickTaskId, session_id: quickSessionId } = await startQuickChatFromSetup(
      quickChat,
      testPage,
    );
    await waitForSessionSettledBaseline(apiClient, quickTaskId, quickSessionId);
    await sendQuickChatMessage(quickChat, testPage, "/slow 30s");
    await waitForAgentMessage(
      apiClient,
      quickSessionId,
      "Running slow response (30s total)",
      30_000,
    );
    expect(await waitForActiveQuickChatSupportsSteering(testPage)).toBe(quickSessionId);
    await waitForActiveQuickChatForegroundActivity(testPage, "generating");

    await testPage.keyboard.press("Control+k");
    const palette = commandDialog(testPage);
    await expect(palette).toBeVisible({ timeout: 10_000 });
    await waitForFiniteAnimations(palette);
    await palette.getByRole("combobox").fill("cancel");
    const cancelOption = palette.getByRole("option").filter({ hasText: /Cancel Turn/i });
    await expect(cancelOption).toHaveCount(1);
    await expect
      .poll(async () => (await cancelOption.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    cancellation.arm(quickSessionId);
    try {
      await cancelOption.tap();
      await waitForQuickChatCancellationPending(testPage, quickSessionId, true);
      await expect(quickChat.getByTestId("cancel-agent-button")).toBeDisabled({ timeout: 5_000 });
    } finally {
      cancellation.release();
    }
    await waitForQuickChatSessionSettled(testPage, quickSessionId);
    await waitForQuickChatDirectInput(quickChat);
    await sendQuickChatMessage(quickChat, testPage, "/e2e:bulk:2");
    await waitForAgentMessage(apiClient, quickSessionId, "Done. Emitted 2 messages", 30_000);
    await expect(quickChat.getByText("Done. Emitted 2 messages", { exact: false })).toBeVisible();
    await prCapture.screenshot("mobile-quick-chat-cancel-palette", {
      caption: "Quick Chat cancellation remains available through the touch-sized command palette",
    });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Quick Chat cancellation palette");
  });
});
