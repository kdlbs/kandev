import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { SessionPage } from "../../pages/session-page";
import { openConversationUsageTask } from "./conversation-usage-helpers";

test("shows usage in the mobile drawer with touch-sized controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(60_000);
  await openConversationUsageTask(testPage, apiClient, seedData, "Mobile conversation usage");

  const statusBar = testPage.getByTestId("chat-status-bar");
  const trigger = statusBar.getByTestId("conversation-usage-trigger");
  await expect(trigger).toBeVisible({ timeout: 15_000 });
  await expect(testPage.getByTestId("conversation-usage-trigger")).toHaveCount(1);
  await expect(trigger).toHaveAccessibleName("Usage");
  await expect(trigger).toHaveText("");
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox).not.toBeNull();
  expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
  expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
  const statusBarBox = await statusBar.boundingBox();
  expect(statusBarBox).not.toBeNull();
  expect(triggerBox!.x).toBeGreaterThanOrEqual(statusBarBox!.x - 1);
  expect(triggerBox!.x + triggerBox!.width).toBeLessThanOrEqual(
    statusBarBox!.x + statusBarBox!.width + 1,
  );
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

test("keeps Usage available in the archived mobile transcript", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(60_000);
  const { taskId } = await openConversationUsageTask(
    testPage,
    apiClient,
    seedData,
    "Archived mobile conversation usage",
  );
  await apiClient.archiveTask(taskId);
  await testPage.goto(`/t/${taskId}`);
  await new SessionPage(testPage).waitForLoad();

  await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();
  const archivedFooter = testPage.getByTestId("archived-chat-footer");
  const trigger = archivedFooter.getByTestId("conversation-usage-trigger");
  await expect(trigger).toBeVisible();
  await expect(trigger).toHaveAccessibleName("Usage");
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox).not.toBeNull();
  expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
  expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
  await trigger.tap();

  const drawer = testPage.getByTestId("conversation-usage-drawer");
  await expect(drawer).toBeVisible();
  await expect(drawer.getByTestId("usage-turn-summary")).toContainText("1,200");
});
