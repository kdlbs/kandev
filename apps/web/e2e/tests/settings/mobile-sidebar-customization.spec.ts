import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

type MobileSidebarLayoutNode = {
  id: string;
  kind: "builtin" | "shortcuts";
  visible: boolean;
  destination_id?: string;
  name?: string;
  shortcuts?: Array<{
    id: string;
    target: { kind: "host_action"; id: "quick_chat" | "quick_terminal" };
  }>;
};

async function seedMobileLayout(
  apiClient: ApiClient,
  workspaceId: string,
  homeVisible = true,
): Promise<void> {
  const current = await apiClient.getUserSettings();
  const expectedRevision =
    current.settings.sidebar_layouts_by_workspace?.[workspaceId]?.revision ?? 0;
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: workspaceId,
      expected_revision: expectedRevision,
      layout: {
        version: 1,
        revision: expectedRevision,
        nodes: [
          { id: "home", kind: "builtin", visible: homeVisible, destination_id: "home" },
          { id: "new-task", kind: "builtin", visible: true, destination_id: "new_task" },
          { id: "automations", kind: "builtin", visible: true, destination_id: "automations" },
          { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
          { id: "integrations", kind: "builtin", visible: true, destination_id: "integrations" },
          {
            id: "group-1",
            kind: "shortcuts",
            visible: true,
            name: "Pinned",
            shortcuts: [
              { id: "chat", target: { kind: "host_action", id: "quick_chat" } },
              { id: "terminal", target: { kind: "host_action", id: "quick_terminal" } },
            ],
          } satisfies MobileSidebarLayoutNode,
        ],
      },
    },
  });
}

async function addMobileShortcut(page: Page, label: string): Promise<void> {
  const picker = page.getByRole("dialog", { name: "Add shortcut" });
  await expect(picker).toBeVisible();
  await picker.getByRole("button", { name: label, exact: true }).click();
}

test.describe("Sidebar customization on phone", () => {
  test("uses the focused editor flow and preserves touch-sized reorder controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedMobileLayout(apiClient, seedData.workspaceId);
    await testPage.goto("/settings/preferences/layouts?tab=sidebar");
    await expect(testPage.getByRole("tab", { name: "Sidebar", exact: true })).toBeVisible();

    const group = testPage.getByTestId("sidebar-layout-node-group-1");
    await group.getByRole("button", { name: "Edit section", exact: true }).tap();
    const focused = testPage.getByTestId("sidebar-layout-focused-group");
    await expect(focused).toBeVisible();

    const moveUp = focused.getByRole("button", { name: "Move up" });
    const moveDown = focused.getByRole("button", { name: "Move down" });
    await expect(moveUp).toHaveCount(2);
    await expect(moveDown).toHaveCount(2);
    for (const button of [moveUp.nth(0), moveDown.nth(0), moveUp.nth(1), moveDown.nth(1)]) {
      const box = await button.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }

    await focused.getByRole("button", { name: "Add shortcut", exact: true }).tap();
    await addMobileShortcut(testPage, "New Task");
    await focused.getByRole("button", { name: "Add shortcut", exact: true }).tap();
    await addMobileShortcut(testPage, "Stats");

    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await saveButton.tap();
    await expect
      .poll(
        async () =>
          (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
            seedData.workspaceId
          ]?.revision,
      )
      .toBeGreaterThan(0);

    await testPage.reload();
    await expect(testPage.getByTestId("sidebar-layout-editor")).toBeVisible();
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});

for (const homeVisible of [true, false]) {
  test(`saved phone layout preserves unified task access with Home visible=${homeVisible}`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedMobileLayout(apiClient, seedData.workspaceId, homeVisible);
    await testPage.goto("/tasks");
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    const layout = menu.getByTestId("mobile-sidebar-layout-navigation");
    await expect(layout).toBeVisible();
    const chat = layout.getByTestId("mobile-quick-chat-button");
    await expect(chat).toBeVisible();
    await expect(layout.getByTestId("mobile-quick-terminal-button")).toBeVisible();
    const home = layout.getByRole("link", { name: "Home", exact: true });
    await expect(home).toHaveCount(homeVisible ? 1 : 0);
    if (homeVisible) {
      await expect(home).toHaveAttribute("aria-current", "page");
      const order = await layout
        .locator("a, button")
        .evaluateAll((nodes) =>
          nodes.map((node) => node.getAttribute("data-testid") ?? node.textContent?.trim()),
        );
      expect(order.indexOf("mobile-quick-chat-button")).toBe(order.indexOf("Home") + 1);
    }
    await expect(menu.getByRole("link", { name: /^(Tasks|Threads)$/ })).toHaveCount(0);
    await expect(menu.getByTestId("mobile-navigation-tasks-toggle")).toBeVisible();
    await expect(menu.getByTestId("mobile-automations-section")).toHaveCount(0);
    const integrations = layout.getByTestId("mobile-shortcut-section-toggle-integrations");
    await expect(integrations).toHaveAttribute("aria-expanded", "false");
    await integrations.tap();
    await expect(integrations).toHaveAttribute("aria-expanded", "true");
    await expect(menu.getByTestId("mobile-customize-sidebar-button")).toBeVisible();
  });
}
