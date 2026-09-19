// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
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
} from "./quick-chat-helpers";

function commandDialog(page: Page) {
  return page.locator('[role="dialog"]:has([cmdk-input])').last();
}

test.describe.serial("Mobile Quick Chat cancellation", () => {
  test.describe.configure({ retries: 1 });

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
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    await testPage.getByTestId("mobile-topbar-menu").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const quickChat = testPage.getByRole("dialog", { name: "Quick Chat" });
    await startQuickChatFromSetup(quickChat, testPage);
    await sendQuickChatMessage(quickChat, testPage, "/slow 30s");

    const quickSessionId = await waitForActiveQuickChatSupportsSteering(testPage);
    await waitForActiveQuickChatForegroundActivity(testPage, "generating");
    const editor = quickChat.locator('.tiptap.ProseMirror[contenteditable="true"]:visible');
    await expect(editor).toHaveText("");
    await expect(quickChat.getByTestId("queue-chip")).not.toBeVisible();
    const cancel = quickChat.getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();
    expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(44);

    await cancel.tap();
    await waitForQuickChatCancellationPending(testPage, quickSessionId, true);
    await expect(cancel).toBeDisabled();
    await waitForQuickChatSessionSettled(testPage, quickSessionId);
    await expect(cancel).not.toBeVisible({ timeout: 15_000 });
  });

  test("mobile Quick Chat exposes one touch cancellation command", async ({
    testPage,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    await testPage.getByTestId("mobile-topbar-menu").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const quickChat = testPage.getByRole("dialog", { name: "Quick Chat" });
    await startQuickChatFromSetup(quickChat, testPage);
    await sendQuickChatMessage(quickChat, testPage, "/slow 30s");
    await expect(
      quickChat.getByRole("status", { name: /Agent is (starting|running)/ }),
    ).toBeVisible({
      timeout: 15_000,
    });

    await testPage.keyboard.press("Control+k");
    const palette = commandDialog(testPage);
    await expect(palette).toBeVisible({ timeout: 10_000 });
    await palette.getByRole("combobox").fill("cancel");
    const cancelOption = palette.getByRole("option").filter({ hasText: /Cancel Turn/i });
    await expect(cancelOption).toHaveCount(1);
    expect((await cancelOption.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await cancelOption.tap();

    await expect(quickChat.getByTestId("cancel-agent-button")).toBeDisabled({ timeout: 5_000 });
    await waitForQuickChatDirectInput(quickChat);
    await prCapture.screenshot("mobile-quick-chat-cancel-palette", {
      caption: "Quick Chat cancellation remains available through the touch-sized command palette",
    });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Quick Chat cancellation palette");
  });
});
