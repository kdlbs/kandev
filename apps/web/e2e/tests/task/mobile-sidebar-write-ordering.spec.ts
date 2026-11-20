import { test, expect } from "../../fixtures/test-base";
import { holdSidebarSettingsWrite } from "../../helpers/user-settings-barrier";
import { touchDragToPoint } from "../../helpers/touch-drag";
import { settledBoundingBox } from "../../helpers/settled-box";
import { watchWs } from "../../helpers/causal-waits";
import type { UserSettingsResponse } from "@/lib/types/http-user-settings";

for (const remoteChange of ["append", "disjoint reorder", "post-dispatch reorder"] as const) {
  test(`touch reorder preserves ${remoteChange}`, async ({
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
      if (remoteChange !== "post-dispatch reorder") {
        await tasks.getByTestId("sidebar-filter-gear").tap();
        const editor = testPage.getByTestId("sidebar-filter-popover");
        await editor.getByTestId("view-rename-button").tap();
        await editor.getByTestId("view-rename-input").fill("R");
        await editor.getByTestId("view-rename-confirm").tap();
        await barrier.arrived;
        await testPage.keyboard.press("Escape");
        await expect(editor).toBeHidden();
      }
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
      if (remoteChange === "post-dispatch reorder") await barrier.arrived;
      const remote = {
        ...initial.views[0],
        id: "remote-append",
        name: "Remote",
      };
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
      const conflict =
        remoteChange === "post-dispatch reorder"
          ? testPage.waitForResponse(
              (response) =>
                response.request().method() === "PATCH" &&
                Boolean(response.request().postDataJSON()?.sidebar_view_state?.view_order),
            )
          : null;
      barrier.release();
      if (conflict) expect((await conflict).status()).toBe(400);
      const middle =
        remoteChange === "post-dispatch reorder"
          ? [
              [first.id, first.name],
              [second.id, second.name],
            ]
          : [
              [second.id, second.name],
              [first.id, "R"],
            ];
      await expect
        .poll(async () => {
          const state = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
            workspace
          ];
          return state.views.map((view) => [view.id, view.name]);
        })
        .toEqual([
          ...initial.views.map((view) => [view.id, view.name]),
          ...middle,
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
          ...middle.map(([id]) => id),
          ...remoteTail.map((view) => view.id),
        ]);
      await expect(tasks.locator(`[data-view-id="${first.id}"]`)).toContainText(
        remoteChange === "post-dispatch reorder" ? first.name : "R",
      );
      await testPage.screenshot({
        path: info.outputPath("phone-rebased-reorder.png"),
        animations: "disabled",
      });
    } finally {
      await barrier.dispose();
      await testPage.close();
      await apiClient.saveUserSettings({
        sidebar_view_state: { workspace_id: workspace, ...initial },
      });
    }
  });
}

test("a rejected dispatched drag cannot return through a queued rename", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  const workspace = seedData.workspaceId;
  const initial = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    workspace
  ];
  const views = ["a", "b", "c", "d"].map((id) => ({
    ...initial.views[0],
    id: `terminal-${id}`,
    name: id.toUpperCase(),
  }));
  const [a, b, c, d] = views;
  const watcher = watchWs(testPage);
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspace,
      views_create: views,
      active_view_id: c.id,
    },
  });
  await testPage.goto(`/?workspaceId=${workspace}`);
  await testPage.getByTestId("app-nav-trigger").tap();
  const tasks = testPage.getByTestId("app-nav-sheet");
  const row = tasks.getByTestId("sidebar-view-chip-row");
  const barrier = await holdSidebarSettingsWrite(testPage);
  try {
    const target = await settledBoundingBox(row.locator(`[data-view-id="${b.id}"]`));
    await touchDragToPoint(testPage, row.locator(`[data-view-id="${a.id}"]`), {
      x: target.x + target.width / 2,
      y: target.y + target.height / 2,
    });
    await barrier.arrived;
    await tasks.getByTestId("sidebar-filter-gear").tap();
    const editor = testPage.getByTestId("sidebar-filter-popover");
    await editor.getByTestId("view-rename-button").tap();
    await editor.getByTestId("view-rename-input").fill("Independent");
    await editor.getByTestId("view-rename-confirm").tap();
    await testPage.keyboard.press("Escape");
    await expect(editor).toBeHidden();
    const remoteOrder = [...initial.views.map((view) => view.id), a.id, c.id, d.id, b.id];
    const delivered = watcher.waitForEvent("user.settings.updated", {
      where: (payload) => {
        const settings = payload as unknown as UserSettingsResponse["settings"];
        return (
          JSON.stringify(
            settings.sidebar_views_by_workspace?.[workspace]?.views.map((view) => view.id),
          ) === JSON.stringify(remoteOrder)
        );
      },
    });
    await apiClient.saveUserSettings({
      sidebar_view_state: { workspace_id: workspace, view_order: remoteOrder },
    });
    await delivered;
    const rejected = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        Boolean(response.request().postDataJSON()?.sidebar_view_state?.view_order),
    );
    const renamed = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        response
          .request()
          .postDataJSON()
          ?.sidebar_view_state?.views_update?.some((view: { id: string }) => view.id === c.id),
    );
    barrier.release();
    expect((await rejected).status()).toBe(400);
    expect((await renamed).status()).toBe(200);
    const persisted = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
      workspace
    ];
    expect(persisted.views.map((view) => view.id)).toEqual(remoteOrder);
    expect(persisted.views.find((view) => view.id === c.id)?.name).toBe("Independent");
    await testPage.reload();
    await testPage.getByTestId("app-nav-trigger").tap();
    await expect
      .poll(() =>
        row
          .getByTestId("sidebar-view-chip")
          .evaluateAll((chips) => chips.map((chip) => chip.getAttribute("data-view-id"))),
      )
      .toEqual(remoteOrder);
    await expect(row.locator(`[data-view-id="${c.id}"]`)).toContainText("Independent");
    await row.scrollIntoViewIfNeeded();
    await testPage.screenshot({
      path: info.outputPath("rejected-move-successor.png"),
      animations: "disabled",
    });
  } finally {
    await barrier.dispose();
    await testPage.close();
    await apiClient.saveUserSettings({
      sidebar_view_state: { workspace_id: workspace, ...initial },
    });
  }
});

test("a committed touch reorder survives a lost response and identical retry", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workspace = seedData.workspaceId;
  const initial = (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
    workspace
  ];
  const a = { ...initial.views[0], id: "retry-a", name: "A" };
  const b = { ...initial.views[0], id: "retry-b", name: "B" };
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: workspace,
      views_create: [a, b],
      active_view_id: a.id,
    },
  });
  let attempts = 0;
  const retried = Promise.withResolvers<number>();
  await testPage.route("**/api/v1/user/settings", async (route) => {
    if (
      route.request().method() !== "PATCH" ||
      !route.request().postDataJSON()?.sidebar_view_state?.view_order_base
    ) {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    attempts++;
    if (attempts === 1) {
      expect(response.status()).toBe(200);
      await route.abort("failed");
    } else {
      retried.resolve(response.status());
      await route.fulfill({ response });
    }
  });
  try {
    await testPage.goto(`/?workspaceId=${workspace}`);
    await testPage.getByTestId("app-nav-trigger").tap();
    const sheet = testPage.getByTestId("app-nav-sheet");
    const row = sheet.getByTestId("sidebar-view-chip-row");
    const box = await settledBoundingBox(row.locator(`[data-view-id="${b.id}"]`));
    await touchDragToPoint(testPage, row.locator(`[data-view-id="${a.id}"]`), {
      x: box.x + box.width / 2,
      y: box.y + box.height / 2,
    });
    expect(await retried.promise).toBe(200);
    const expected = [...initial.views.map((view) => view.id), b.id, a.id];
    await expect
      .poll(async () =>
        (await apiClient.getUserSettings()).settings.sidebar_views_by_workspace[
          workspace
        ].views.map((view) => view.id),
      )
      .toEqual(expected);
    await testPage.reload();
    await testPage.getByTestId("app-nav-trigger").tap();
    await expect
      .poll(() =>
        sheet
          .getByTestId("sidebar-view-chip-row")
          .getByTestId("sidebar-view-chip")
          .evaluateAll((chips) => chips.map((chip) => chip.getAttribute("data-view-id"))),
      )
      .toEqual(expected);
    expect(attempts).toBe(2);
  } finally {
    await testPage.close();
    await apiClient.saveUserSettings({
      sidebar_view_state: { workspace_id: workspace, ...initial },
    });
  }
});
