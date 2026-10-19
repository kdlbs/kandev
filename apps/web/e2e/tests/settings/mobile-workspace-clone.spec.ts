import { test, expect } from "../../fixtures/test-base";
import { seedCloneSource, proveClone } from "../../helpers/workspace-clone";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

test("phone clone uses an inset sheet and persists the complete setup", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const source = await seedCloneSource(apiClient, seedData.repositoryPath);
  await testPage.setViewportSize({ width: 390, height: 844 });
  await testPage.goto("/settings/workspaces");
  const trigger = testPage.getByRole("button", { name: `Actions for ${source.name}`, exact: true });
  await trigger.scrollIntoViewIfNeeded();
  const triggerBounds = await trigger.boundingBox();
  expect(triggerBounds!.height).toBeGreaterThanOrEqual(44);
  expect(triggerBounds!.width).toBeGreaterThanOrEqual(44);
  expect(triggerBounds!.width).toBeLessThanOrEqual(48);
  const headingBounds = await testPage
    .getByRole("heading", { name: source.name, exact: true })
    .boundingBox();
  expect(
    Math.abs(
      triggerBounds!.y + triggerBounds!.height / 2 - headingBounds!.y - headingBounds!.height / 2,
    ),
  ).toBeLessThan(2);
  await trigger.tap();
  const cloneItem = testPage.getByRole("menuitem", { name: "Clone workspace", exact: true });
  await expect(cloneItem).toBeVisible();
  await waitForFiniteAnimations(testPage.getByRole("menu"));
  expect((await cloneItem.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  const menuBox = await testPage.getByRole("menu").boundingBox();
  expect(menuBox!.x).toBeGreaterThanOrEqual(0);
  expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(390);
  await cloneItem.tap();
  const sheet = testPage.getByTestId("workspace-clone-sheet");
  await expect(sheet).toBeVisible();
  await expect(sheet).toContainText("repository secrets");
  await expect(sheet).toContainText("Personal GitHub sign-in stays separate.");
  const box = await sheet.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(391);
  expect(box!.height).toBeLessThan(844 * 0.8);
  const input = sheet.getByLabel("Workspace Name", { exact: true });
  const submit = sheet.getByRole("button", { name: "Clone workspace", exact: true });
  expect((await input.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect((await submit.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await sheet.getByRole("button", { name: "Cancel", exact: true }).tap();
  await expect(sheet).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.tap();
  await testPage.getByRole("menuitem", { name: "Clone workspace", exact: true }).tap();
  await input.fill("My cloned workspace");
  const created = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/workspaces/${source.id}/clone`) &&
      response.request().method() === "POST",
  );
  await submit.tap();
  const response = await created;
  expect(response.status()).toBe(201);
  const target = await response.json();
  await expect(testPage).toHaveURL(new RegExp(`/settings/workspaces/${target.id}$`));
  await expect(testPage.getByTestId("workspace-settings-active-badge")).toHaveCount(0);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await proveClone(testPage, apiClient, source.id, target.id, true);
});

test("phone failure retains the name and prevents duplicate pending taps", async ({
  testPage,
  apiClient,
}) => {
  const source = await apiClient.createWorkspace(
    "Retry source with a long name for the phone sheet",
  );
  await testPage.goto("/settings/workspaces");
  await testPage.getByRole("button", { name: `Actions for ${source.name}`, exact: true }).tap();
  await testPage.getByRole("menuitem", { name: "Clone workspace", exact: true }).tap();
  const sheet = testPage.getByTestId("workspace-clone-sheet");
  await sheet.getByLabel("Workspace Name", { exact: true }).fill("   ");
  await expect(sheet.getByRole("button", { name: "Clone workspace", exact: true })).toBeDisabled();
  await sheet.getByLabel("Workspace Name", { exact: true }).fill("My cloned workspace");
  let requests = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const path = `**/api/v1/workspaces/${source.id}/clone`;
  await testPage.route(path, async (route) => {
    requests++;
    await gate;
    await route.fulfill({ status: 503, json: { error: "injected failure" } });
  });
  const submit = sheet.getByRole("button", { name: "Clone workspace", exact: true });
  const request = testPage.waitForRequest((req) =>
    req.url().endsWith(`/workspaces/${source.id}/clone`),
  );
  await submit.tap();
  await request;
  await expect(
    sheet.getByRole("button", { name: "Cloning workspace...", exact: true }),
  ).toBeDisabled();
  await testPage
    .getByRole("button", { name: "Cloning workspace...", exact: true })
    .dispatchEvent("click");
  expect(requests).toBe(1);
  release();
  await expect(sheet.getByRole("alert")).toBeVisible();
  await expect(sheet.getByLabel("Workspace Name", { exact: true })).toHaveValue(
    "My cloned workspace",
  );
  await testPage.unroute(path);
  const created = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/workspaces/${source.id}/clone`) && response.status() === 201,
  );
  await submit.tap();
  await created;
  await expect(sheet).toBeHidden();
});

test("phone workspace page actions fit the header and clone the viewed source", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const source = await seedCloneSource(apiClient, seedData.repositoryPath);
  await testPage.setViewportSize({ width: 320, height: 844 });
  await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/workflows`);
  const activeActions = testPage
    .getByTestId("workspace-settings-shell")
    .getByTestId("workspace-actions-menu");
  await expect(testPage.getByTestId("workspace-settings-active-badge")).toBeVisible();
  const activeBox = await activeActions.boundingBox();
  expect(activeBox!.x + activeBox!.width).toBeLessThanOrEqual(320);
  expect(activeBox!.width).toBeGreaterThanOrEqual(44);
  await activeActions.tap();
  await expect(
    testPage.getByRole("menuitem", { name: "Clone workspace", exact: true }),
  ).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(activeActions).toBeFocused();
  await testPage.setViewportSize({ width: 390, height: 844 });
  await testPage.goto(`/settings/workspaces/${source.id}/workflows`);
  const trigger = testPage.getByRole("button", { name: `Actions for ${source.name}`, exact: true });
  const box = await trigger.boundingBox();
  expect(box!.height).toBeGreaterThanOrEqual(44);
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.x + box!.width).toBeLessThanOrEqual(390);
  await trigger.tap();
  const cloneItem = testPage.getByRole("menuitem", { name: "Clone workspace", exact: true });
  await expect(cloneItem).toBeVisible();
  await waitForFiniteAnimations(testPage.getByRole("menu"));
  expect((await cloneItem.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await cloneItem.tap();
  const sheet = testPage.getByTestId("workspace-clone-sheet");
  await expect(sheet).toBeVisible();
  await expect(sheet).toContainText(source.name);
  await sheet.getByRole("button", { name: "Cancel", exact: true }).tap();
  await expect(trigger).toBeFocused();
  await trigger.tap();
  await testPage.getByRole("menuitem", { name: "Clone workspace", exact: true }).tap();
  await sheet.getByLabel("Workspace Name", { exact: true }).fill("My cloned workspace");
  const created = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/workspaces/${source.id}/clone`) &&
      response.request().method() === "POST",
  );
  await sheet.getByRole("button", { name: "Clone workspace", exact: true }).tap();
  const response = await created;
  expect(response.status()).toBe(201);
  const target = await response.json();
  await expect(testPage).toHaveURL(new RegExp(`/settings/workspaces/${target.id}$`));
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await proveClone(testPage, apiClient, source.id, target.id, true);
});
