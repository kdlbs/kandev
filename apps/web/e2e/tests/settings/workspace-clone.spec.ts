import { test, expect } from "../../fixtures/test-base";
import { seedCloneSource, proveClone } from "../../helpers/workspace-clone";

test("clones full setup and query defaults without changing the active workspace", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const source = await seedCloneSource(apiClient, seedData.repositoryPath);
  await testPage.goto("/settings/workspaces");
  const card = testPage
    .getByTestId("workspace-list-item")
    .filter({ has: testPage.getByRole("heading", { name: source.name, exact: true }) });
  await testPage.setViewportSize({ width: 2048, height: 900 });
  const trigger = card.getByRole("button", { name: `Actions for ${source.name}`, exact: true });
  const triggerBounds = await trigger.boundingBox();
  expect(triggerBounds!.width).toBeCloseTo(28, 0);
  expect(triggerBounds!.height).toBeCloseTo(28, 0);
  const headingBounds = await card
    .getByRole("heading", { name: source.name, exact: true })
    .boundingBox();
  expect(triggerBounds!.x - headingBounds!.x - headingBounds!.width).toBeLessThanOrEqual(16);
  await trigger.click();
  const cloneItem = testPage.getByRole("menuitem", { name: "Clone workspace", exact: true });
  await expect(cloneItem).toHaveCSS("height", "28px");
  await cloneItem.click();
  const dialog = testPage.getByTestId("workspace-clone-dialog");
  await expect(dialog).toBeVisible();
  await expect(testPage).toHaveURL(/\/settings\/workspaces$/);
  const bounds = await dialog.boundingBox();
  expect(bounds!.width).toBeLessThanOrEqual(450);
  await dialog.getByLabel("Workspace Name", { exact: true }).fill("My cloned workspace");
  const created = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/workspaces/${source.id}/clone`) &&
      response.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "Clone workspace", exact: true }).click();
  const response = await created;
  expect(response.status()).toBe(201);
  const target = await response.json();
  await expect(testPage).toHaveURL(new RegExp(`/settings/workspaces/${target.id}$`));
  await testPage.getByTestId("workspace-settings-switcher").click();
  await expect(
    testPage.getByTestId(`workspace-settings-switcher-item-${seedData.workspaceId}`),
  ).toContainText("Active");
  await testPage.keyboard.press("Escape");
  await proveClone(testPage, apiClient, source.id, target.id);
});

test("cancel returns focus and resize preserves an unsaved clone name", async ({
  testPage,
  apiClient,
}) => {
  const source = await apiClient.createWorkspace("Resize source");
  await testPage.goto("/settings/workspaces");
  const trigger = testPage.getByRole("button", { name: `Actions for ${source.name}`, exact: true });
  for (const width of [767, 768, 1280]) {
    await testPage.setViewportSize({ width, height: 800 });
    const size = width < 768 ? "44px" : "28px";
    await expect(trigger).toHaveCSS("width", size);
    await expect(trigger).toHaveCSS("height", size);
  }
  await trigger.focus();
  await testPage.keyboard.press("Enter");
  await testPage.getByRole("menuitem", { name: "Clone workspace", exact: true }).click();
  await testPage.getByLabel("Workspace Name", { exact: true }).fill("Unsaved copy");
  await testPage.setViewportSize({ width: 390, height: 844 });
  await expect(testPage.getByTestId("workspace-clone-sheet")).toBeVisible();
  await expect(testPage.getByLabel("Workspace Name", { exact: true })).toHaveValue("Unsaved copy");
  await testPage.setViewportSize({ width: 1280, height: 800 });
  const dialog = testPage.getByTestId("workspace-clone-dialog");
  await expect(dialog.getByLabel("Workspace Name", { exact: true })).toHaveValue("Unsaved copy");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trigger).toBeFocused();
  await expect(testPage).toHaveURL(/\/settings\/workspaces$/);
});

test("workspace page actions clone the viewed workspace from any tab", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const source = await seedCloneSource(apiClient, seedData.repositoryPath);
  await testPage.goto(`/settings/workspaces/${source.id}/repositories`);
  const trigger = testPage.getByRole("button", { name: `Actions for ${source.name}`, exact: true });
  await expect(trigger).toBeVisible();
  await trigger.click();
  await testPage.getByRole("menuitem", { name: "Clone workspace", exact: true }).click();
  const dialog = testPage.getByTestId("workspace-clone-dialog");
  await expect(dialog).toContainText(source.name);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await testPage.getByRole("menuitem", { name: "Clone workspace", exact: true }).click();
  await dialog.getByLabel("Workspace Name", { exact: true }).fill("My cloned workspace");
  const created = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/workspaces/${source.id}/clone`) &&
      response.request().method() === "POST",
  );
  await dialog.getByRole("button", { name: "Clone workspace", exact: true }).click();
  const response = await created;
  expect(response.status()).toBe(201);
  const target = await response.json();
  await expect(testPage).toHaveURL(new RegExp(`/settings/workspaces/${target.id}$`));
  await proveClone(testPage, apiClient, source.id, target.id);
});
