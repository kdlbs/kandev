/**
 * E2E: the plugin task panel / kanban Edit submenu / card indicator hooks
 * added by this task (docs/plans/plugins/PLUGIN-API.md's registerTaskPanel /
 * registerTaskMenuAction / the "task-card-indicators" slot).
 *
 * Uses the same real `plugin-fixture` gRPC plugin package as
 * `plugins.spec.ts` — see that file's header for how
 * apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz is built
 * (`make -C apps/backend e2e-plugin-package`). The fixture's `ui/bundle.js`
 * registers a "Notes" task panel (mobile-enabled), a `task-card-indicators`
 * slot component, and an "edit"-group kanban menu action — see
 * apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js.
 */
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";
import type { ApiClient } from "../../helpers/api-client";

const PANEL_ID = "notes";

async function uninstallViaApi(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

test.describe("Plugins — task panel / kanban Edit submenu / card indicator", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallViaApi(apiClient);
  });

  test("registerTaskPanel: opens from the + menu, round-trips through host.storage, and survives a reload (AC1, AC3, AC19)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await installFixturePlugin(testPage);

    // A repo-backed task with an agent (not a bare createTask) so the task
    // gets a real environment id — the dockview per-env layout persistence
    // this test exercises (AC3) is keyed by envId and is a no-op without one,
    // same as every other layout-persistence-across-reload spec.
    const seedTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plugin panel seed task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${seedTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // --- AC1: the "+" menu shows a "Notes" row; opening it renders the
    // plugin's Component as a real dockview panel. ---
    await session.addPanelButton().click();
    const addPanelRow = session.addPanelPluginItem(PLUGIN_ID, PANEL_ID);
    await expect(addPanelRow).toBeVisible();
    await expect(addPanelRow).toHaveText(/Notes/);
    await addPanelRow.click();

    const notesEditor = testPage.getByTestId("e2e-notes-panel");
    await expect(notesEditor).toBeVisible({ timeout: 10_000 });
    await expect(notesEditor).toHaveValue("");

    // --- AC19: host.storage.set/get round-trip through the real backend
    // (the fixture debounces ~150ms before writing). ---
    await notesEditor.fill("hello from e2e");
    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/plugins/${PLUGIN_ID}/user-state/task/${seedTask.id}/note`,
          );
          if (res.status !== 200) return null;
          const body = (await res.json()) as { value: string };
          return body.value;
        },
        { timeout: 10_000, intervals: [250, 500, 1000] },
      )
      .toBe("hello from e2e");

    // --- AC3: reloading restores the panel at the same id/title/position —
    // the layout was saved with the open plugin panel. Wait on the restored
    // panel directly rather than session.waitForLoad() (which gates on the
    // chat panel specifically) — the agent-backed environment can take a
    // moment to reconnect post-reload, and that reconnection isn't what
    // this assertion is about. ---
    await testPage.reload();
    await expect(testPage.getByTestId("e2e-notes-panel")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("e2e-notes-panel")).toHaveValue("hello from e2e", {
      timeout: 10_000,
    });
  });

  test("disabling the plugin closes its open panel and removes the + menu row (AC4)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await installFixturePlugin(testPage);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "Plugin panel disable task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${seedTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.addPanelButton().click();
    await session.addPanelPluginItem(PLUGIN_ID, PANEL_ID).click();
    await expect(testPage.getByTestId("e2e-notes-panel")).toBeVisible({ timeout: 10_000 });

    const consoleErrors: string[] = [];
    testPage.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });

    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible({ timeout: 10_000 });

    await testPage.goto(`/t/${seedTask.id}`);
    await session.waitForLoad();
    await expect(testPage.getByTestId("e2e-notes-panel")).toHaveCount(0);
    await session.addPanelButton().click();
    await expect(session.addPanelPluginItem(PLUGIN_ID, PANEL_ID)).toHaveCount(0);
    expect(consoleErrors).toEqual([]);
  });

  test("registerTaskMenuAction: the kanban card Edit item becomes a submenu, and the plugin action runs (AC9)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await installFixturePlugin(testPage);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "Plugin edit menu task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto("/");
    const kanban = new KanbanPage(testPage);
    await expect(kanban.board).toBeVisible({ timeout: 15_000 });

    await kanban.taskCard(seedTask.id).click({ button: "right" });
    const editSubmenu = testPage.getByTestId("kanban-edit-submenu");
    await expect(editSubmenu).toBeVisible();
    await editSubmenu.click();

    await expect(testPage.getByRole("menuitem", { name: "Edit task" })).toBeVisible();
    const pluginAction = testPage.getByRole("menuitem", { name: "Enhance notes" });
    await expect(pluginAction).toBeVisible();
    await pluginAction.click();

    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/plugins/${PLUGIN_ID}/user-state/task/${seedTask.id}/note`,
          );
          if (res.status !== 200) return null;
          const body = (await res.json()) as { value: string };
          return body.value;
        },
        { timeout: 10_000, intervals: [250, 500, 1000] },
      )
      .toBe("Enhanced via plugin action");

    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/plugins/${PLUGIN_ID}/user-state/task/${seedTask.id}/menu-presentation`,
          );
          if (res.status !== 200) return null;
          const body = (await res.json()) as { value: string };
          return body.value;
        },
        { timeout: 10_000, intervals: [250, 500, 1000] },
      )
      .toBe("desktop");
  });

  test("a sibling surface's write (the kanban action's default writerId) still reaches an open task panel in the same tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Regression test for review feedback: host.storage.subscribe's echo
    // suppression used to compare against one writer id shared by every
    // surface in the tab, so the task panel's own subscription treated the
    // kanban "Enhance notes" action's write (a different surface, same tab)
    // as its own echo and silently dropped it. The panel now subscribes
    // under its own panelId (see the fixture's useNotesValue), so a write
    // carrying any other writerId — exactly what the kanban action's
    // fire-and-forget default write looks like — must still come through.
    await installFixturePlugin(testPage);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "Sibling surface sync task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${seedTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.addPanelButton().click();
    await session.addPanelPluginItem(PLUGIN_ID, PANEL_ID).click();
    const notesEditor = testPage.getByTestId("e2e-notes-panel");
    await expect(notesEditor).toBeVisible({ timeout: 10_000 });
    await expect(notesEditor).toHaveValue("");

    // Simulates the kanban "Enhance notes" action's write — same endpoint,
    // same request shape (host.storage.set with no writerId override) — but
    // triggered directly rather than via the kanban board UI, since the
    // board and an open task panel are different routes and can't be
    // on-screen at the same time in this app's navigation model. What
    // matters for this regression is that the write's writerId does NOT
    // match the open panel's own panelId, exactly as a sibling surface's
    // write would look from the panel's point of view.
    const res = await apiClient.rawRequest(
      "PUT",
      `/api/plugins/${PLUGIN_ID}/user-state/task/${seedTask.id}/note`,
      { value: "Enhanced via plugin action", writerId: "e2e-sibling-surface" },
    );
    expect(res.status).toBe(200);

    await expect(notesEditor).toHaveValue("Enhanced via plugin action", { timeout: 10_000 });
  });

  test("useNotesValue discards a stale storage read that resolves after a newer one", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Regression test for review feedback: the fixture's useNotesValue only
    // guarded against unmount (`cancelled`), not against two overlapping
    // host.storage.get calls resolving out of order — the mount's initial
    // read and a subscribe-triggered refresh from another surface's write.
    // If the mount's read (fetched first, before the other write) resolves
    // *after* the refresh triggered by that write, it must not overwrite the
    // newer value with its own stale one.
    await installFixturePlugin(testPage);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "Stale read task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const notePath = `/api/plugins/${PLUGIN_ID}/user-state/task/${seedTask.id}/note`;

    let firstRequestSeen = false;
    let releaseFirst: () => void = () => {};
    const firstHeld = new Promise<void>((resolve) => {
      releaseFirst = resolve;
    });

    await testPage.route(`**${notePath}`, async (route) => {
      if (route.request().method() !== "GET" || firstRequestSeen) {
        return route.continue();
      }
      firstRequestSeen = true;
      await firstHeld;
      // Fulfilled with canned stale content instead of passed through, so
      // its body is deterministically the pre-update value even though the
      // real store has since moved on.
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ value: "first note", updatedAt: new Date(0).toISOString() }),
      });
    });

    await testPage.goto(`/t/${seedTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.addPanelButton().click();
    await session.addPanelPluginItem(PLUGIN_ID, PANEL_ID).click();

    // The mount's GET is held — the panel stays on its loading state, but
    // its subscription (set up in the same effect) is already live.
    await expect(testPage.getByTestId("e2e-notes-panel-loading")).toBeVisible();

    // A sibling surface's write lands; its notification triggers a second,
    // unheld refresh that resolves quickly with the real current value.
    const res = await apiClient.rawRequest("PUT", notePath, {
      value: "second note",
      writerId: "e2e-sibling-surface",
    });
    expect(res.status).toBe(200);

    const notesEditor = testPage.getByTestId("e2e-notes-panel");
    await expect(notesEditor).toHaveValue("second note", { timeout: 10_000 });

    // Now let the stale first read land — it must not revert the panel.
    releaseFirst();
    await testPage.waitForTimeout(500);
    await expect(notesEditor).toHaveValue("second note");
  });

  test("task-card-indicators slot renders the plugin's indicator on the kanban card (AC13)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await installFixturePlugin(testPage);

    const seedTask = await apiClient.createTask(
      seedData.workspaceId,
      "Plugin card indicator task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    await testPage.goto("/");
    const kanban = new KanbanPage(testPage);
    await expect(kanban.board).toBeVisible({ timeout: 15_000 });

    const indicator = kanban.taskCard(seedTask.id).getByTestId("e2e-card-indicator");
    await expect(indicator).toBeVisible({ timeout: 15_000 });
    await expect(indicator).toHaveAttribute("data-task-id", seedTask.id);
  });
});
