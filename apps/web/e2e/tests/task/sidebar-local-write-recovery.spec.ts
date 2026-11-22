import { test, expect } from "../../fixtures/test-base";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";

test("a failed active-view change cannot leave a phantom draft after a no-op successor", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  const workspace = seedData.workspaceId;
  const initial = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    workspace
  ];
  const base = { ...initial.views[0], id: "settle-base", name: "Confirmed view" };
  const other = { ...initial.views[0], id: "settle-other", name: "Uncommitted view" };
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspace,
      views_create: [base, other],
      active_view_id: base.id,
    },
  });
  const arrived = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  let held = false;
  await testPage.route("**/api/v1/user/settings", async (route) => {
    if (
      route.request().method() === "PATCH" &&
      route.request().postDataJSON()?.sidebar_view_state?.active_view_id === other.id &&
      !held
    ) {
      held = true;
      arrived.resolve();
      await release.promise;
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "write refused before commit" }),
      });
      return;
    }
    await route.continue();
  });
  try {
    await testPage.goto(`/?workspaceId=${workspace}`);
    const editor = new SidebarFilterPopoverPage(testPage);
    await editor.selectViewByName(other.name);
    await arrived.promise;
    await editor.open();
    const successor = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        Boolean(response.request().postDataJSON()?.sidebar_view_state?.draft_update),
    );
    await editor.setSort("Title");
    release.resolve();
    expect((await successor).status()).toBe(200);
    await expect(testPage.getByTestId("tasks-view-picker")).toContainText(base.name);
    const persisted = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
      workspace
    ];
    expect(persisted.active_view_id).toBe(base.id);
    expect(persisted.draft).toBeNull();
    await editor.close();
    await testPage.reload();
    await expect(testPage.getByTestId("tasks-view-picker")).toContainText(base.name);
    await testPage.screenshot({
      path: info.outputPath("confirmed-local-state.png"),
      animations: "disabled",
    });
  } finally {
    release.resolve();
    await testPage.close();
    await apiClient.saveUserSettings({
      sidebar_view_state: { workspace_id: workspace, ...initial },
    });
  }
});
