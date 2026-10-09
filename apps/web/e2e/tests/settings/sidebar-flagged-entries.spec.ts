import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  enableCoordinatorFeature,
  readCoordinatorFeature,
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

async function savedNodeVisible(apiClient: ApiClient, workspaceId: string, nodeId: string) {
  const settings = await apiClient.getUserSettings();
  return settings.settings.sidebar_layouts_by_workspace?.[workspaceId]?.nodes.find(
    (node) => node.id === nodeId,
  )?.visible;
}

async function savedNodeIndex(apiClient: ApiClient, workspaceId: string, nodeId: string) {
  const settings = await apiClient.getUserSettings();
  return (
    settings.settings.sidebar_layouts_by_workspace?.[workspaceId]?.nodes.findIndex(
      (node) => node.id === nodeId,
    ) ?? -1
  );
}

test("customizes Coordinator independently and retains its choice while the feature is off", async ({
  testPage: page,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const releaseFeature = await enableCoordinatorFeature(backend, apiClient, seedData.workspaceId);
  let releasedInitialFeature = false;
  let releaseExplicitOff: (() => Promise<void>) | undefined;
  let releasedExplicitOff = false;
  let releaseReturnedFeature: (() => Promise<void>) | undefined;

  try {
    await resetSidebar(apiClient, seedData.workspaceId);
    await page.goto("/tasks");
    const sidebar = page.getByTestId("app-sidebar");
    const coordinatorNode = sidebar.getByTestId("sidebar-node-coordinators");
    await expect(coordinatorNode).toBeVisible();
    await expect(sidebar.getByTestId("sidebar-coordinators-empty")).toBeVisible();
    await expect(sidebar.getByTestId("coordinators-open-list-body")).toBeVisible();
    await expect(sidebar.getByTestId("coordinators-open-list")).toHaveCount(0);

    await coordinatorNode.click({ button: "right" });
    const menu = page.getByTestId("sidebar-customize-menu");
    await expect(menu.getByTestId("sidebar-visibility-coordinators")).toBeVisible();
    await menu.getByTestId("sidebar-visibility-coordinators").click();
    await expect
      .poll(() => savedNodeVisible(apiClient, seedData.workspaceId, "coordinators"))
      .toBe(false);
    await expect(sidebar.getByTestId("sidebar-node-coordinators")).toHaveCount(0);

    await sidebar.getByTestId("sidebar-node-home").click({ button: "right" });
    await expect(menu.getByTestId("sidebar-visibility-automations")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await menu.getByTestId("sidebar-visibility-automations").click();
    await expect(sidebar.getByTestId("sidebar-node-automations")).toHaveCount(0);
    await expect(sidebar.getByTestId("sidebar-node-coordinators")).toHaveCount(0);
    await sidebar.getByTestId("sidebar-node-home").click({ button: "right" });
    await menu.getByTestId("sidebar-visibility-automations").click();
    await expect(sidebar.getByTestId("sidebar-node-automations")).toBeVisible();

    await sidebar.getByTestId("sidebar-node-home").click({ button: "right" });
    await menu.getByTestId("sidebar-visibility-coordinators").click();
    await expect(menu).toBeHidden();
    await expect(sidebar.getByTestId("sidebar-node-coordinators")).toBeVisible();
    await expect(sidebar.getByTestId("sidebar-coordinators-empty")).toBeVisible();
    await sidebar.getByTestId("sidebar-node-home").click({ button: "right" });
    await expect(menu).toBeVisible();
    await menu.getByRole("menuitemcheckbox", { name: "Show fast action icons" }).click();
    await expect(sidebar.getByTestId("coordinators-open-list")).toBeVisible();
    await expect(sidebar.getByTestId("coordinators-open-list-body")).toHaveCount(0);

    await sidebar.getByTestId("sidebar-node-home").click({ button: "right" });
    await menu.getByRole("menuitem", { name: "Sidebar layout settings" }).click();
    await expect(page).toHaveURL(/\/settings\/preferences\/layouts\?tab=sidebar$/);
    const coordinatorRow = page.getByTestId("sidebar-layout-node-coordinators");
    await coordinatorRow.getByRole("button", { name: "Move down", exact: true }).click();
    const save = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(save).toBeEnabled();
    await save.click();
    await expect
      .poll(async () => {
        const coordinatorIndex = await savedNodeIndex(
          apiClient,
          seedData.workspaceId,
          "coordinators",
        );
        const automationsIndex = await savedNodeIndex(
          apiClient,
          seedData.workspaceId,
          "automations",
        );
        return coordinatorIndex > automationsIndex;
      })
      .toBe(true);
    await page.reload();
    await expect(page.getByTestId("sidebar-layout-node-coordinators")).toBeVisible();

    await page.goto("/tasks");
    await sidebar.getByTestId("sidebar-node-home").click({ button: "right" });
    await menu.getByTestId("sidebar-visibility-coordinators").click();
    await expect
      .poll(() => savedNodeVisible(apiClient, seedData.workspaceId, "coordinators"))
      .toBe(false);
    await page.reload();
    await expect(sidebar.getByTestId("sidebar-node-coordinators")).toHaveCount(0);

    await releaseFeature();
    releasedInitialFeature = true;
    releaseExplicitOff = await backend.useEnv({ KANDEV_FEATURES_COORDINATOR: "false" });
    expect(await readCoordinatorFeature(apiClient)).toBe(false);
    await page.goto("/settings/preferences/layouts?tab=sidebar");
    const unavailableCoordinator = page.getByTestId("sidebar-layout-node-coordinators");
    const unavailableSwitch = unavailableCoordinator.getByRole("switch", {
      name: "Toggle visibility",
    });
    await expect(unavailableCoordinator).toContainText("Unavailable");
    await expect(unavailableSwitch).toBeDisabled();
    await expect(unavailableSwitch).not.toBeChecked();

    await releaseExplicitOff();
    releasedExplicitOff = true;
    releaseReturnedFeature = await enableCoordinatorFeature(
      backend,
      apiClient,
      seedData.workspaceId,
    );
    await page.reload();
    const returnedSwitch = page
      .getByTestId("sidebar-layout-node-coordinators")
      .getByRole("switch", { name: "Toggle visibility" });
    await expect(returnedSwitch).toBeEnabled();
    await expect(returnedSwitch).not.toBeChecked();
    expect(await savedNodeVisible(apiClient, seedData.workspaceId, "coordinators")).toBe(false);
    expect(await savedNodeIndex(apiClient, seedData.workspaceId, "coordinators")).toBeGreaterThan(
      await savedNodeIndex(apiClient, seedData.workspaceId, "automations"),
    );
    await page.goto("/tasks");
    await expect(sidebar.getByTestId("sidebar-node-coordinators")).toHaveCount(0);
  } finally {
    if (releaseReturnedFeature) await releaseReturnedFeature();
    else if (releaseExplicitOff && !releasedExplicitOff) await releaseExplicitOff();
    else if (!releasedInitialFeature) await releaseFeature();
  }
});
