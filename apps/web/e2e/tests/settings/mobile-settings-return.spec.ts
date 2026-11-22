import { test, expect } from "../../fixtures/test-base";
import { waitForStableActiveSession } from "../../helpers/session-store";

// @covers AC-UI-SETTINGS-RETURN-001.1, AC-UI-SETTINGS-RETURN-001.2, AC-UI-SETTINGS-RETURN-001.3, AC-UI-SETTINGS-RETURN-001.6
test("returns to the originating task from the phone settings header", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.seedTask(seedData.workspaceId, "Phone Settings return", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.seedTaskSession(task.task_id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
  });
  const selected = await apiClient.seedTaskSession(task.task_id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
  });
  await apiClient.seedSessionMessage(selected.session_id, {
    type: "message",
    authorType: "agent",
    content: "Phone selected session marker",
  });
  const href = `/t/${task.task_id}?sessionId=${selected.session_id}`;
  await testPage.goto(href);
  await waitForStableActiveSession(testPage, selected.session_id);
  await expect(testPage.getByText("Phone selected session marker")).toBeVisible();
  await testPage.evaluate(() => Reflect.set(window, "settingsReturnDocument", "phone-document"));
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  await menu.getByRole("link", { name: "Settings", exact: true }).tap();
  await expect(testPage).toHaveURL(/\/settings$/);
  const index = testPage.getByTestId("settings-index");
  await index.locator('a[href="/settings/preferences/appearance"]').first().tap();
  await expect(testPage).toHaveURL(/\/settings\/preferences\/appearance$/);

  const close = testPage.getByRole("button", { name: "Close settings", exact: true });
  const target = await close.boundingBox();
  expect(target).not.toBeNull();
  expect(target!.width).toBeGreaterThanOrEqual(44);
  expect(target!.height).toBeGreaterThanOrEqual(44);
  const header = await close.locator("xpath=ancestor::header").boundingBox();
  expect(target!.y).toBeGreaterThanOrEqual(header!.y);
  expect(target!.y + target!.height).toBeLessThanOrEqual(header!.y + header!.height);

  await testPage.screenshot({ path: test.info().outputPath("settings-close-phone.png") });

  const saved = await apiClient.getUserSettings();
  const nextLayout = saved.settings.changes_panel_layout === "flat" ? "Tree" : "Flat list";
  const layout = testPage.getByTestId("changes-panel-layout-select");
  await layout.tap();
  await testPage.getByRole("option", { name: nextLayout, exact: true }).tap();
  await close.tap();
  const dialog = testPage.getByRole("alertdialog", { name: "Save changes before leaving?" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Continue editing" }).tap();
  await expect(testPage).toHaveURL(/\/settings\/preferences\/appearance$/);
  await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
  await close.tap();
  await dialog.getByRole("button", { name: "Discard and leave" }).tap();

  await expect(testPage).toHaveURL(href);
  await waitForStableActiveSession(testPage, selected.session_id);
  await expect(testPage.getByText("Phone selected session marker")).toBeVisible();
  expect(await testPage.evaluate(() => Reflect.get(window, "settingsReturnDocument"))).toBe(
    "phone-document",
  );
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
    true,
  );
});

// @covers AC-UI-SETTINGS-RETURN-001.4, AC-UI-SETTINGS-RETURN-001.6
test("keeps the phone close target usable at the narrow fine-pointer boundary", async ({
  desktopTestPage: page,
}) => {
  await page.setViewportSize({ width: 767, height: 851 });
  await page.goto("/settings");
  const close = page.getByRole("button", { name: "Close settings", exact: true });
  await expect(close).toBeVisible();
  const box = await close.boundingBox();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await close.click();
  await expect(page).not.toHaveURL(/\/settings/);
});
