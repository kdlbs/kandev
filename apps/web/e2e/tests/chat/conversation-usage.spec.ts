import { expect, test } from "../../fixtures/test-base";
import { openConversationUsageTask } from "./conversation-usage-helpers";

test("shows turn and session usage in the desktop popover", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(60_000);
  await openConversationUsageTask(testPage, apiClient, seedData, "Conversation usage details");

  const trigger = testPage.getByTestId("conversation-usage-trigger");
  await expect(trigger).toBeVisible({ timeout: 15_000 });
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await trigger.press("Enter");
  await expect(trigger).toHaveAttribute("aria-expanded", "true");

  const popover = testPage.getByTestId("conversation-usage-popover");
  await expect(popover).toBeVisible();
  await expect(popover.getByTestId("usage-turn-summary")).toContainText("1,200");
  await expect(popover).toContainText("Estimated cost");
  await expect(popover).toContainText("$1.23");
  await expect(popover).toContainText("Last response");
  await expect(popover.getByTestId("usage-last-response")).toContainText("80");
  await expect(popover).toContainText("Session recorded total");
  await expect(popover).toContainText("Estimated cost");
  await expect(popover).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(popover).toBeHidden();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await trigger.click();
  await expect(popover).toBeVisible();
  await prCapture.screenshot("conversation-usage-desktop", {
    caption: "Desktop conversation footer with the usage entry point",
  });
});
