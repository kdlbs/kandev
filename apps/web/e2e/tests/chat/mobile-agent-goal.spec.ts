import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { routeSessionEntryRecovery } from "../../helpers/session-entry-recovery";
import {
  startQuickChatFromSetup,
  sendQuickChatMessage,
  waitForQuickChatDirectInput,
} from "./quick-chat-helpers";

async function openMobileQuickChat(page: Page): Promise<Locator> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  await page.getByTestId("app-nav-trigger").tap();
  await page.getByTestId("mobile-quick-chat-button").tap();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

test.describe("mobile agent goal visibility", () => {
  test.describe.configure({ retries: 0 });

  test("submits once while the message acknowledgement is delayed", async ({ testPage }) => {
    const proxy = await routeSessionEntryRecovery(testPage);
    const dialog = await openMobileQuickChat(testPage);
    await startQuickChatFromSetup(dialog, testPage);
    await waitForQuickChatDirectInput(dialog);
    await expect(
      dialog.getByText("Please get ready for my next question.", { exact: false }).first(),
    ).toBeVisible();
    // The opening prompt can be carried by launch for passthrough profiles or
    // by message.add for structured profiles. The settled conversation above
    // is the baseline; this test measures only the following user submission.
    const openingMessageRequestCount = proxy.requestCount("message.add");
    proxy.delayNextResponses("message.add", 1, 3_500, "exercise asynchronous composer clearing");

    await sendQuickChatMessage(dialog, testPage, "/e2e:goal-active");

    await expect.poll(() => proxy.delayedResponseCount("message.add"), { timeout: 15_000 }).toBe(1);
    await expect
      .poll(() => proxy.requestCount("message.add"), { timeout: 15_000 })
      .toBe(openingMessageRequestCount + 1);
    await expect(dialog.getByTestId("agent-goal-chip")).toBeVisible();
  });

  test("opens the active goal in a touch drawer and clears it", async ({ testPage, prCapture }) => {
    const dialog = await openMobileQuickChat(testPage);
    await startQuickChatFromSetup(dialog, testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:goal-active");

    const chip = dialog.getByTestId("agent-goal-chip");
    await expect(chip).toBeVisible({ timeout: 30_000 });
    await expect(
      dialog
        .getByText("The provider goal remains active after the thread becomes idle.", {
          exact: false,
        })
        .last(),
    ).toBeVisible({ timeout: 30_000 });
    const bounds = await chip.boundingBox();
    expect(bounds).not.toBeNull();
    if (!bounds) throw new Error("goal trigger bounds unavailable");
    expect(bounds.width).toBeGreaterThanOrEqual(44);
    expect(bounds.height).toBeGreaterThanOrEqual(44);

    await chip.tap();
    const drawer = testPage.getByTestId("agent-goal-drawer-content");
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("Coordinate contributor PR reviews");
    await expect(drawer).toContainText("The agent may continue automatically between replies.");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile agent goal drawer");

    const close = testPage.getByRole("button", { name: "Close goal details" });
    const closeBounds = await close.boundingBox();
    expect(closeBounds).not.toBeNull();
    if (!closeBounds) throw new Error("goal close bounds unavailable");
    expect(closeBounds.width).toBeGreaterThanOrEqual(44);
    expect(closeBounds.height).toBeGreaterThanOrEqual(44);

    await close.tap();
    await expect(drawer).toBeHidden();

    await waitForQuickChatDirectInput(dialog);
    await sendQuickChatMessage(dialog, testPage, "/e2e:goal-complete");
    await expect(dialog.getByText("The provider goal is complete.", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(dialog.getByTestId("agent-goal-chip")).toBeHidden({ timeout: 15_000 });

    await waitForQuickChatDirectInput(dialog);
    await sendQuickChatMessage(dialog, testPage, "/e2e:goal-active");
    const activeGoalChip = dialog.getByTestId("agent-goal-chip");
    await expect(activeGoalChip).toBeVisible({ timeout: 30_000 });
    await activeGoalChip.tap();
    const activeGoalDrawer = testPage.getByTestId("agent-goal-drawer-content");
    await expect(activeGoalDrawer).toBeVisible();
    await expect(activeGoalDrawer.getByTestId("agent-goal-objective")).toContainText(
      "Coordinate contributor PR reviews",
    );
    await waitForFiniteAnimations(activeGoalDrawer);
    await prCapture.screenshot("phone-agent-goal", {
      caption: "Phone goal drawer with a touch close control",
    });
    await testPage.getByRole("button", { name: "Close goal details" }).tap();
    await expect(activeGoalDrawer).toBeHidden();

    await waitForQuickChatDirectInput(dialog);
    await sendQuickChatMessage(dialog, testPage, "/e2e:goal-clear");
    await expect(
      dialog.getByText("The provider goal was cleared.", { exact: false }),
    ).toBeVisible();
    await waitForQuickChatDirectInput(dialog);
    await expect(dialog.getByTestId("agent-goal-chip")).toBeHidden({ timeout: 15_000 });
  });
});
