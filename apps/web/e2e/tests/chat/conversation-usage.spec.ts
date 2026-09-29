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

  const statusBar = testPage.getByTestId("chat-status-bar");
  const trigger = statusBar.getByTestId("conversation-usage-trigger");
  await expect(trigger).toBeVisible({ timeout: 15_000 });
  await expect(testPage.getByTestId("conversation-usage-trigger")).toHaveCount(1);
  await expect(trigger).toHaveAccessibleName("Usage");
  await expect(trigger).toHaveText("");
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox).not.toBeNull();
  expect(triggerBox!.width).toBe(24);
  expect(triggerBox!.height).toBe(24);
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  const tooltip = testPage.getByRole("tooltip");
  await trigger.focus();
  await expect(tooltip).toHaveText("Usage");
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
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await trigger.hover();
  await expect(tooltip).toHaveText("Usage");
  await trigger.click();
  await expect(popover).toBeVisible();
  await prCapture.screenshot("conversation-usage-desktop", {
    caption: "Desktop composer status row with the usage entry point",
  });
});

// @covers AC-COSTS-CONVERSATION-USAGE-003.3
test("uses a touch-sized Usage drawer at a narrow fine-pointer viewport", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(60_000);
  await openConversationUsageTask(
    testPage,
    apiClient,
    seedData,
    "Narrow window conversation usage",
  );
  await testPage.setViewportSize({ width: 500, height: 900 });

  await expect
    .poll(() => testPage.evaluate(() => window.matchMedia("(pointer: fine)").matches))
    .toBe(true);
  const statusBar = testPage.getByTestId("chat-status-bar");
  const trigger = statusBar.getByTestId("conversation-usage-trigger");
  await expect(trigger).toBeVisible({ timeout: 15_000 });
  await expect(trigger).toHaveAccessibleName("Usage");
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox).not.toBeNull();
  expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
  expect(triggerBox!.height).toBeGreaterThanOrEqual(44);

  await trigger.click();
  const drawer = testPage.getByTestId("conversation-usage-drawer");
  await expect(drawer).toBeVisible();
  await expect(drawer).toContainText("1,200");
  await expect(testPage.getByTestId("conversation-usage-popover")).toHaveCount(0);
});
