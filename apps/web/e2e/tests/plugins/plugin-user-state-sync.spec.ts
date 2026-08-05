/**
 * E2E: host.storage realtime sync (Approach F1) — a write in one browser
 * context reaches a second context's subscription for the same document,
 * and the writing context never sees its own echo (AC24, AC25).
 *
 * The e2e harness runs with auth disabled (KANDEV_E2E_MOCK=true), so every
 * browser context resolves to the same synthetic default user — exactly the
 * same-user, two-surface case the plan calls out (a second tab/device, or
 * the kanban Edit shortcut and the task panel both editing one document).
 * The cross-user negative (a *different* user gets nothing) is a Go-level
 * test — see internal/gateway/websocket/user_notifications_test.go.
 */
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

async function openSecondContext(
  testPage: Page,
  frontendUrl: string,
  backendPort: number,
): Promise<Page> {
  const context = await testPage.context().browser()!.newContext({ baseURL: frontendUrl });
  const page = await context.newPage();
  await page.addInitScript(
    ({ port }: { port: number }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = String(port);
    },
    { port: backendPort },
  );
  return page;
}

test.describe("Plugins — host.storage realtime sync", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("a write in one tab updates the other tab's subscription without an echo on the writer (AC24, AC25)", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // --- Install once; both contexts share the same backend/session. ---
    await installFixturePlugin(testPage);

    const seedTask = await apiClient.createTask(seedData.workspaceId, "Sync spec task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // --- Tab A: open the task, open the Notes panel. ---
    await testPage.goto(`/t/${seedTask.id}`);
    const sessionA = new SessionPage(testPage);
    await sessionA.waitForLoad();
    await sessionA.addPanelButton().click();
    await sessionA.addPanelPluginItem(PLUGIN_ID, "notes").click();
    const notesA = testPage.getByTestId("e2e-notes-panel");
    await expect(notesA).toBeVisible({ timeout: 10_000 });

    // --- Tab B: a second browser context, same (synthetic) user, same task. ---
    const pageB = await openSecondContext(testPage, backend.frontendUrl, backend.port);
    try {
      await pageB.goto(`/t/${seedTask.id}`);
      const sessionB = new SessionPage(pageB);
      await sessionB.waitForLoad();
      await sessionB.addPanelButton().click();
      await sessionB.addPanelPluginItem(PLUGIN_ID, "notes").click();
      const notesB = pageB.getByTestId("e2e-notes-panel");
      await expect(notesB).toBeVisible({ timeout: 10_000 });

      // --- AC24: typing in Tab A propagates to Tab B via
      // plugin.user-state.updated, with no manual reload/poll in Tab B. ---
      await notesA.fill("written from tab A");
      await expect(notesB).toHaveValue("written from tab A", { timeout: 10_000 });

      // --- AC25: Tab A's own selection/caret is untouched by its own write —
      // the host suppresses the writing tab's own echo via writerId, so the
      // controlled input never gets a redundant re-set from a self-notification
      // racing the local keystroke. Prove it by continuing to type in Tab A
      // immediately after the debounced save and getting the expected result,
      // not a value that reverted or duplicated mid-edit. ---
      await notesA.fill("written from tab A, continued");
      await expect(notesA).toHaveValue("written from tab A, continued");
      await expect(notesB).toHaveValue("written from tab A, continued", { timeout: 10_000 });

      // --- And the reverse direction: Tab B's write reaches Tab A. ---
      await notesB.fill("written from tab B");
      await expect(notesA).toHaveValue("written from tab B", { timeout: 10_000 });
    } finally {
      await pageB.context().close();
    }
  });
});
