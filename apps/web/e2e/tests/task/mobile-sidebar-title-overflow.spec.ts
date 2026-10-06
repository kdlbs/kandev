import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";

const LONG_TITLE = "Investigate sidebar title overflow without losing the ending";

function taskRow(surface: Locator, taskId: string) {
  return surface.locator(`[data-task-row-id="${taskId}"]`);
}

function titleElement(row: Locator) {
  return row.locator("span.overflow-hidden").first();
}

async function maskImage(title: Locator) {
  return title.evaluate((element) => {
    const style = getComputedStyle(element);
    return style.maskImage || style.webkitMaskImage;
  });
}

test("mobile task picker fades clipped titles and keeps row navigation and actions usable", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await testPage.addInitScript(() => localStorage.setItem("theme", "light"));
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const longTask = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, stepOptions);
  const shortTask = await apiClient.createTask(
    seedData.workspaceId,
    "Short phone title",
    stepOptions,
  );
  const navigationTask = await apiClient.createTask(
    seedData.workspaceId,
    "Mobile title picker navigation",
    stepOptions,
  );

  await testPage.goto(`/t/${navigationTask.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const trigger = testPage.getByTestId("mobile-task-picker-trigger");
  await expect(trigger).toBeVisible();
  await trigger.tap();

  const picker = testPage.getByRole("dialog", { name: "Tasks" });
  await expect(picker).toBeVisible();
  const list = picker.getByTestId("mobile-task-switcher-list");
  const longRow = taskRow(list, longTask.id);
  const shortRow = taskRow(list, shortTask.id);
  const longTitle = titleElement(longRow);
  const shortTitle = titleElement(shortRow);

  await expect(longTitle).toHaveAttribute("data-truncated", "true");
  await expect(shortTitle).toHaveAttribute("data-truncated", "false");
  await expect(longTitle).toHaveText(LONG_TITLE);
  await expect.poll(() => maskImage(longTitle)).toContain("linear-gradient");
  await expect.poll(() => maskImage(shortTitle)).toBe("none");
  await expect(list).toHaveCSS("overflow-y", "auto");

  const [pickerBox, listBox, rowBox] = await Promise.all([
    picker.boundingBox(),
    list.boundingBox(),
    longRow.boundingBox(),
  ]);
  expect(pickerBox).not.toBeNull();
  expect(listBox).not.toBeNull();
  expect(rowBox).not.toBeNull();
  expect(listBox!.x).toBeGreaterThanOrEqual(pickerBox!.x);
  expect(listBox!.x + listBox!.width).toBeLessThanOrEqual(pickerBox!.x + pickerBox!.width);
  expect(rowBox!.x).toBeGreaterThanOrEqual(listBox!.x);
  expect(rowBox!.x + rowBox!.width).toBeLessThanOrEqual(listBox!.x + listBox!.width);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);

  const action = longRow.locator("button.mobile-task-actions-button");
  await expect(action).toBeVisible();
  const actionBox = await action.boundingBox();
  expect(actionBox).not.toBeNull();
  expect(actionBox!.width).toBeGreaterThanOrEqual(44);
  expect(actionBox!.height).toBeGreaterThanOrEqual(44);
  await prCapture.screenshot("mobile-sidebar-title-overflow-picker", {
    caption: "Phone task picker with a faded long title, visible actions, and short title",
  });

  await longRow.tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${longTask.id}(?:\\?|$)`));
  await expect(picker).toBeHidden();
  await expect(trigger).toContainText(LONG_TITLE);
});
