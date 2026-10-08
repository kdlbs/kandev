import { expect, test } from "../../fixtures/test-base";
import type { BrowserContext, Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import {
  deleteCoordinator,
  enableCoordinatorFeature,
  seedCoordinator,
} from "../coordinator/coordinator-fixture";

async function resetSidebar(apiClient: ApiClient, workspaceId: string) {
  const current = await apiClient.getUserSettings();
  await apiClient.saveUserSettings({
    sidebar_fast_actions_enabled: false,
    sidebar_new_task_style: "simple",
    sidebar_layout_state: {
      workspace_id: workspaceId,
      expected_revision:
        current.settings.sidebar_layouts_by_workspace?.[workspaceId]?.revision ?? 0,
      layout: null,
    },
  });
}

async function savedNode(apiClient: ApiClient, workspaceId: string, nodeId: string) {
  const settings = await apiClient.getUserSettings();
  return settings.settings.sidebar_layouts_by_workspace?.[workspaceId]?.nodes.find(
    (node) => node.id === nodeId,
  );
}

async function expectPhoneTargets(page: Page) {
  const navigation = page.getByTestId("mobile-sidebar-layout-navigation");
  const coordinators = navigation.getByTestId("mobile-coordinators-section");
  const header = coordinators.getByRole("button").first();
  expect(Math.round((await header.boundingBox())?.height ?? 0)).toBeGreaterThanOrEqual(44);
  const bodyLink = coordinators.getByTestId("mobile-coordinators-open-list-body");
  expect(Math.round((await bodyLink.boundingBox())?.height ?? 0)).toBeGreaterThanOrEqual(44);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

async function expectNarrowFinePointer(page: Page, backendUrl: string) {
  const browser = page.context().browser();
  if (!browser) throw new Error("Browser context is required for the fine-pointer check");
  const context: BrowserContext = await browser.newContext({
    baseURL: backendUrl,
    viewport: { width: 560, height: 900 },
    isMobile: false,
    hasTouch: false,
    storageState: await page.context().storageState(),
  });
  try {
    const backendPort = await page.evaluate(() => window.__KANDEV_API_PORT);
    await context.addInitScript(
      ({ apiPort }: { apiPort: string }) => {
        window.__KANDEV_API_PORT = apiPort;
      },
      { apiPort: backendPort },
    );
    const finePage = await context.newPage();
    await finePage.goto("/tasks");
    expect(await finePage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
    await finePage.getByTestId("app-nav-trigger").click();
    await expect(finePage.getByTestId("mobile-coordinators-section")).toBeVisible();
    await expectPhoneTargets(finePage);
    await finePage.getByTestId("mobile-sidebar-customize").click();
    const drawer = finePage.getByTestId("mobile-sidebar-customization");
    const move = drawer.getByRole("button", { name: "Move Coordinators down", exact: true });
    expect(Math.round((await move.boundingBox())?.height ?? 0)).toBeGreaterThanOrEqual(44);
    const bounds = (await drawer.boundingBox())!;
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(finePage.viewportSize()!.width);
  } finally {
    await context.close();
  }
}

test("customizes Coordinator from the phone drawer and saves the same layout in Settings", async ({
  testPage: page,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const releaseFeature = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);
  let coordinatorId: string | undefined;
  try {
    await resetSidebar(apiClient, seedData.workspaceId);
    await page.goto("/tasks");
    await page.getByTestId("app-nav-trigger").tap();
    const navigation = page.getByTestId("mobile-sidebar-layout-navigation");
    const coordinators = navigation.getByTestId("mobile-coordinators-section");
    await expect(coordinators).toBeVisible();
    await expect(coordinators.getByTestId("mobile-sidebar-coordinators-empty")).toBeVisible();
    await expect(coordinators.getByTestId("mobile-coordinators-open-list-body")).toBeVisible();
    await expect(coordinators.getByTestId("mobile-coordinators-open-list")).toHaveCount(0);
    await expectPhoneTargets(page);

    const seeded = await seedCoordinator(apiClient, seedData.workspaceId, {
      name: "Phone sidebar coordinator",
      agentProfileId: seedData.agentProfileId,
      executorProfileId: seedData.worktreeExecutorProfileId,
    });
    coordinatorId = seeded.id;
    await page.goto("/tasks");
    await page.getByTestId("app-nav-trigger").tap();
    await expect(page.getByTestId(`mobile-sidebar-coordinator-${seeded.id}`)).toBeVisible();
    await page.getByTestId(`mobile-sidebar-coordinator-${seeded.id}`).tap();
    await expect(page).toHaveURL(
      new RegExp(`/workspaces/${seedData.workspaceId}/coordinator/${seeded.id}$`),
    );

    await page.goto("/tasks");
    await page.getByTestId("app-nav-trigger").tap();
    await page.getByTestId("mobile-sidebar-customize").tap();
    const drawer = page.getByTestId("mobile-sidebar-customization");
    const visibility = drawer.getByTestId("mobile-sidebar-visibility-coordinators");
    await expect(visibility).toBeChecked();
    const checkboxLabel = visibility.locator("xpath=ancestor::label[1]");
    expect(Math.round((await checkboxLabel.boundingBox())?.height ?? 0)).toBeGreaterThanOrEqual(44);
    await visibility.tap();
    await expect
      .poll(async () => (await savedNode(apiClient, seedData.workspaceId, "coordinators"))?.visible)
      .toBe(false);
    await expect(visibility).not.toBeChecked();
    await visibility.tap();
    await expect
      .poll(async () => (await savedNode(apiClient, seedData.workspaceId, "coordinators"))?.visible)
      .toBe(true);
    await drawer.getByRole("button", { name: "Move Coordinators down", exact: true }).tap();
    await expect
      .poll(async () => {
        const settings = await apiClient.getUserSettings();
        const nodes =
          settings.settings.sidebar_layouts_by_workspace?.[seedData.workspaceId]?.nodes ?? [];
        return (
          nodes.findIndex((node) => node.id === "coordinators") >
          nodes.findIndex((node) => node.id === "automations")
        );
      })
      .toBe(true);

    await drawer.getByRole("button", { name: "Sidebar layout settings" }).tap();
    await expect(page).toHaveURL(/\/settings\/preferences\/layouts\?tab=sidebar$/);
    const coordinatorRow = page.getByTestId("sidebar-layout-node-coordinators");
    await coordinatorRow.getByRole("switch", { name: "Toggle visibility" }).tap();
    const save = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(save).toBeEnabled();
    await save.tap();
    await expect
      .poll(async () => (await savedNode(apiClient, seedData.workspaceId, "coordinators"))?.visible)
      .toBe(false);
    await expect
      .poll(async () => {
        const node = await savedNode(apiClient, seedData.workspaceId, "coordinators");
        return node?.id;
      })
      .toBe("coordinators");

    await page.goto("/tasks");
    await page.getByTestId("app-nav-trigger").tap();
    await expect(page.getByTestId("mobile-coordinators-section")).toHaveCount(0);
    await page.getByTestId("mobile-sidebar-customize").tap();
    const restoreDrawer = page.getByTestId("mobile-sidebar-customization");
    const restore = restoreDrawer.getByTestId("mobile-sidebar-visibility-coordinators");
    await expect(restore).not.toBeChecked();
    await restore.tap();
    await expect
      .poll(async () => (await savedNode(apiClient, seedData.workspaceId, "coordinators"))?.visible)
      .toBe(true);
    await restoreDrawer.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(page.getByTestId("mobile-coordinators-section")).toBeVisible();

    await expectNarrowFinePointer(page, backend.frontendUrl);
  } finally {
    if (coordinatorId) await deleteCoordinator(apiClient, seedData.workspaceId, coordinatorId);
    await releaseFeature();
  }
});
