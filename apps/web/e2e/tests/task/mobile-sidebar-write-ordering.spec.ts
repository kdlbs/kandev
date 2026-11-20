import { test, expect } from "../../fixtures/test-base";
import { holdSidebarSettingsWrite } from "../../helpers/user-settings-barrier";
import { touchDragToPoint } from "../../helpers/touch-drag";
import { settledBoundingBox } from "../../helpers/settled-box";

test("queued touch reorder retains an acknowledged remote append and local rename", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  test.setTimeout(120_000);
  const workspace = seedData.workspaceId;
  const initial = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    workspace
  ];
  const first = { ...initial.views[0], id: "queued-a", name: "A" };
  const second = { ...initial.views[0], id: "queued-b", name: "B" };
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspace,
      views_create: [first, second],
      active_view_id: first.id,
    },
  });
  await testPage.goto(`/?workspaceId=${workspace}`);
  await testPage.getByTestId("app-nav-trigger").tap();
  const tasks = testPage.getByTestId("app-nav-sheet");
  const barrier = await holdSidebarSettingsWrite(testPage);
  try {
    await tasks.getByTestId("sidebar-filter-gear").tap();
    const editor = testPage.getByTestId("sidebar-filter-popover");
    await editor.getByTestId("view-rename-button").tap();
    await editor.getByTestId("view-rename-input").fill("Renamed");
    await editor.getByTestId("view-rename-confirm").tap();
    await barrier.arrived;
    await testPage.keyboard.press("Escape");
    await expect(editor).toBeHidden();
    const row = tasks.getByTestId("sidebar-view-chip-row");
    const source = row.locator(`[data-view-id="${first.id}"]`);
    const target = row.locator(`[data-view-id="${second.id}"]`);
    const targetBox = await settledBoundingBox(target);
    await touchDragToPoint(testPage, source, {
      x: targetBox.x + targetBox.width / 2,
      y: targetBox.y + targetBox.height / 2,
    });
    const expected = [...initial.views.map((view) => view.id), second.id, first.id];
    await expect
      .poll(() =>
        row
          .getByTestId("sidebar-view-chip")
          .evaluateAll((chips) => chips.map((chip) => chip.getAttribute("data-view-id"))),
      )
      .toEqual(expected);
    const remote = { ...initial.views[0], id: "remote-append", name: "Remote" };
    await apiClient.saveUserSettings({
      sidebar_view_state: { workspace_id: workspace, views_create: [remote] },
    });
    barrier.release();
    await expect
      .poll(async () => {
        const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
          workspace
        ];
        return state.views.map((view) => [view.id, view.name]);
      })
      .toEqual([
        ...initial.views.map((view) => [view.id, view.name]),
        [second.id, second.name],
        [first.id, "Renamed"],
        [remote.id, remote.name],
      ]);
    await testPage.reload();
    await testPage.getByTestId("app-nav-trigger").tap();
    await expect
      .poll(() =>
        tasks
          .getByTestId("sidebar-view-chip-row")
          .getByTestId("sidebar-view-chip")
          .evaluateAll((chips) => chips.map((chip) => chip.getAttribute("data-view-id"))),
      )
      .toEqual([...expected, remote.id]);
    await expect(tasks.locator(`[data-view-id="${first.id}"]`)).toContainText("Renamed");
    await testPage.screenshot({
      path: info.outputPath("phone-rebased-reorder.png"),
      animations: "disabled",
    });
  } finally {
    await barrier.dispose();
  }
});
