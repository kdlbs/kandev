import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

type SidebarLayoutNode = {
  id: string;
  kind: "builtin" | "shortcuts";
  visible: boolean;
  destination_id?: string;
  name?: string;
  shortcuts?: Array<{
    id: string;
    target: { kind: "host_action" | "destination" | "automation"; id: string };
  }>;
};

function defaultNodes(groups: SidebarLayoutNode[]): SidebarLayoutNode[] {
  return [
    { id: "home", kind: "builtin", visible: true, destination_id: "home" },
    { id: "new-task", kind: "builtin", visible: true, destination_id: "new_task" },
    { id: "automations", kind: "builtin", visible: true, destination_id: "automations" },
    { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
    { id: "integrations", kind: "builtin", visible: true, destination_id: "integrations" },
    ...groups,
  ];
}

async function saveSidebarLayout(
  apiClient: ApiClient,
  workspaceId: string,
  nodes: SidebarLayoutNode[],
): Promise<number> {
  const current = await apiClient.getUserSettings();
  const expectedRevision =
    current.settings.sidebar_layouts_by_workspace?.[workspaceId]?.revision ?? 0;
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: workspaceId,
      expected_revision: expectedRevision,
      layout: { version: 1, revision: expectedRevision, nodes },
    },
  });
  return expectedRevision + 1;
}

async function addShortcut(page: Page, label: string) {
  const picker = page.getByRole("dialog", { name: "Add shortcut" });
  await expect(picker).toBeVisible();
  await picker.getByRole("button", { name: label, exact: true }).click();
}

test.describe("Sidebar customization on desktop", () => {
  test("opens from the Sidebar tab on the Layouts settings page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await saveSidebarLayout(apiClient, seedData.workspaceId, defaultNodes([]));

    await testPage.goto("/settings/preferences/layouts");
    await expect(testPage.getByRole("tab", { name: "Sidebar", exact: true })).toBeVisible();
    await testPage.getByRole("tab", { name: "Sidebar", exact: true }).click();

    await expect(testPage).toHaveURL(/\/settings\/preferences\/layouts\?tab=sidebar$/);
    await expect(testPage.getByTestId("sidebar-layout-editor")).toBeVisible();
  });

  test("adds four shortcuts, hides a destination, reorders a group, and persists the result", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const initialRevision = await saveSidebarLayout(
      apiClient,
      seedData.workspaceId,
      defaultNodes([
        { id: "group-1", kind: "shortcuts", visible: true, name: "Pinned", shortcuts: [] },
        { id: "group-2", kind: "shortcuts", visible: true, name: "More", shortcuts: [] },
      ]),
    );

    await testPage.goto("/settings/sidebar");
    const editor = testPage.getByTestId("sidebar-layout-editor");
    const group = testPage.getByTestId("sidebar-layout-node-group-1");
    const add = group.getByRole("button", { name: "Add shortcut", exact: true });

    await add.click();
    await addShortcut(testPage, "New Task");
    await add.click();
    await addShortcut(testPage, "Quick Chat");
    await add.click();
    await addShortcut(testPage, "Quick terminal");
    await add.click();
    await addShortcut(testPage, "Stats");

    const homeNode = testPage.getByTestId("sidebar-layout-node-home");
    await homeNode.getByRole("switch", { name: "Toggle visibility" }).click();
    await expect(testPage.getByTestId("sidebar-layout-drag-handle-group-1")).toBeVisible();
    await testPage
      .getByTestId("sidebar-layout-node-group-1")
      .getByRole("button", { name: "Move down", exact: true })
      .first()
      .click();

    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(saveButton).toBeEnabled();
    await saveButton.click();
    await expect
      .poll(
        async () =>
          (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
            seedData.workspaceId
          ]?.revision,
      )
      .toBe(initialRevision + 1);

    await testPage.reload();
    await expect(editor).toBeVisible();
    for (const label of ["New Task", "Quick Chat", "Quick terminal", "Stats"]) {
      await expect(testPage.getByTestId("sidebar-layout-node-group-1")).toContainText(label);
    }
    await expect(
      testPage
        .getByTestId("sidebar-layout-node-home")
        .getByRole("switch", { name: "Toggle visibility" }),
    ).not.toBeChecked();

    const saved = await apiClient.getUserSettings();
    const nodes = (saved.settings.sidebar_layouts_by_workspace?.[seedData.workspaceId]?.nodes ??
      []) as Array<{
      id: string;
    }>;
    expect(nodes.slice(-2).map((node) => node.id)).toEqual(["group-2", "group-1"]);

    await testPage.goto("/");
    const section = testPage.getByRole("button", { name: /Pinned/ });
    await expect(section).toBeVisible();
    await section.click();
    await expect(
      testPage.getByRole("button", { name: "Quick Chat", exact: true }).last(),
    ).toBeVisible();
  });

  test("updates a pinned automation bubble when a run starts", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Pinned activity",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await saveSidebarLayout(
      apiClient,
      seedData.workspaceId,
      defaultNodes([
        {
          id: "group-1",
          kind: "shortcuts",
          visible: true,
          name: "Pinned",
          shortcuts: [
            { id: "automation-shortcut", target: { kind: "automation", id: automation.id } },
          ],
        },
      ]),
    );

    await testPage.goto("/");
    const shortcut = testPage.getByTestId("sidebar-shortcut-automation-shortcut");
    const indicator = shortcut.getByTestId("sidebar-shortcut-activity-indicator");
    await expect(indicator).toHaveAttribute("data-state", "idle", { timeout: 15_000 });

    await apiClient.seedAutomationRun(automation.id, "triggered");
    await expect(indicator).toHaveAttribute("data-state", "running", { timeout: 15_000 });
  });
});
