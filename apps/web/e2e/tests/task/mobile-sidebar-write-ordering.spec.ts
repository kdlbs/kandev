import { test, expect } from "../../fixtures/test-base";
import { holdSidebarSettingsWrite } from "../../helpers/user-settings-barrier";
import { touchDragToPoint } from "../../helpers/touch-drag";
import { settledBoundingBox } from "../../helpers/settled-box";

for (const remoteChange of ["append", "disjoint reorder"] as const) {
  test(`queued touch reorder preserves ${remoteChange} and local rename`, async ({
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
    const tail =
      remoteChange === "append"
        ? []
        : [
            { ...initial.views[0], id: "queued-c", name: "C" },
            { ...initial.views[0], id: "queued-d", name: "D" },
          ];
    await apiClient.saveUserSettings({
      sidebar_view_state: {
        workspace_id: workspace,
        views_create: [first, second, ...tail],
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
      await editor.getByTestId("view-rename-input").fill("R");
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
      const expected = [
        ...initial.views.map((view) => view.id),
        second.id,
        first.id,
        ...tail.map((view) => view.id),
      ];
      await expect
        .poll(() =>
          row
            .getByTestId("sidebar-view-chip")
            .evaluateAll((chips) => chips.map((chip) => chip.getAttribute("data-view-id"))),
        )
        .toEqual(expected);
      const remote = { ...initial.views[0], id: "remote-append", name: "Remote" };
      const remoteTail = remoteChange === "append" ? [remote] : [...tail].reverse();
      await apiClient.saveUserSettings({
        sidebar_view_state:
          remoteChange === "append"
            ? { workspace_id: workspace, views_create: [remote] }
            : {
                workspace_id: workspace,
                view_order: [
                  ...initial.views.map((view) => view.id),
                  first.id,
                  second.id,
                  ...remoteTail.map((view) => view.id),
                ],
              },
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
          [first.id, "R"],
          ...remoteTail.map((view) => [view.id, view.name]),
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
        .toEqual([
          ...initial.views.map((view) => view.id),
          second.id,
          first.id,
          ...remoteTail.map((view) => view.id),
        ]);
      await expect(tasks.locator(`[data-view-id="${first.id}"]`)).toContainText("R");
      await testPage.screenshot({
        path: info.outputPath("phone-rebased-reorder.png"),
        animations: "disabled",
      });
    } finally {
      await barrier.dispose();
    }
  });
}
