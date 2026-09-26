import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { openConversationUsageTask } from "./conversation-usage-helpers";

test("shows usage in the mobile drawer with touch-sized controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(60_000);
  await openConversationUsageTask(testPage, apiClient, seedData, "Mobile conversation usage");

  const trigger = testPage.getByTestId("conversation-usage-trigger");
  await expect(trigger).toBeVisible({ timeout: 15_000 });
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox).not.toBeNull();
  expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
  expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
  await trigger.tap();

  const drawerScroll = testPage.getByTestId("conversation-usage-drawer");
  await expect(drawerScroll).toBeVisible();
  await expect(drawerScroll).toContainText("1,200");
  await expect(drawerScroll).toContainText("Last response");
  await expect(drawerScroll).toContainText("Estimated cost");
  await expect(drawerScroll).toContainText("Session recorded total");
  await expect(drawerScroll).toHaveCSS("overflow-y", "auto");
  await waitForFiniteAnimations(drawerScroll);
  await prCapture.screenshot("mobile-conversation-usage", {
    caption: "Mobile usage drawer with turn and session detail",
  });

  const hasHorizontalOverflow = await testPage.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollWidth > root.clientWidth + 1;
  });
  expect(hasHorizontalOverflow).toBe(false);

  await drawerScroll.getByRole("button", { name: "Close" }).first().tap();
  await expect(trigger).toBeFocused();
});
