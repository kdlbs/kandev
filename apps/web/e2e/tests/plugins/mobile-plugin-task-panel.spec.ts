// Routing: /t/{taskId}. File starts with "mobile-" so it runs on the
// mobile-chrome Playwright project (Pixel 5 emulation) — see
// mobile-file-viewer.spec.ts's header for the convention.
//
// E2E: a mobileEnabled task panel (registerTaskPanel) gets a phone
// bottom-nav entry, and selecting it renders the plugin's Component
// full-width in the mobile panel area (AC7). Uses the same real
// `plugin-fixture` package as plugin-task-panel.spec.ts.
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test.describe("Mobile plugin task panel", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("bottom-nav entry opens the plugin's Component full-width (AC7)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Install through the real Settings > Plugins upload flow (reachable on
    // a phone via the settings menu sheet, same as mobile-plugin-nav.spec.ts).
    await installFixturePlugin(testPage);

    // A repo-backed task with an agent (not a bare createTask) so the task
    // has a real session — the mobile bottom-nav panel switch this test
    // exercises is a no-op without one (handlePanelChange in
    // use-session-layout-state.ts guards on effectiveSessionId), same as
    // every other mobile-nav e2e spec (see mobile-terminal-keybar.spec.ts).
    const seedTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile plugin panel task",
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

    // Mobile groups all plugin panels behind one bounded bottom-nav action.
    const panelsNavButton = testPage.getByRole("button", { name: "Panels" });
    await expect(panelsNavButton).toBeVisible({ timeout: 15_000 });
    expect((await panelsNavButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await panelsNavButton.tap();
    const notesOption = testPage.getByTestId("mobile-plugin-panel-option-kandev-plugin-e2e-notes");
    await expect(notesOption).toBeVisible({ timeout: 10_000 });
    expect((await notesOption.boundingBox())?.height).toBeGreaterThanOrEqual(44);

    const notesEditor = testPage.getByTestId("e2e-notes-panel");
    // On mobile the bottom-nav button can be tapped before hydration wires
    // its handler; a lost tap leaves the panel unmounted (see
    // mobile-terminal-helpers.ts's tapTerminalTab/switchToTerminalPanel for
    // the same pattern). Re-tap once if the first tap didn't take.
    await notesOption.tap();
    if (!(await notesEditor.isVisible())) {
      await panelsNavButton.tap();
      await notesOption.tap();
    }
    await expect(notesEditor).toBeVisible({ timeout: 10_000 });
    await expect(notesEditor).toHaveAttribute("data-presentation", "mobile");

    // host.storage round-trips the same way it does on desktop.
    await notesEditor.fill("hello from mobile e2e");
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
      .toBe("hello from mobile e2e");

    // Definitive disable while focused selects Chat and removes the grouped
    // plugin action rather than leaving a dead panel selection behind.
    await testPage.goto("/settings/plugins");
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible({ timeout: 10_000 });
    await testPage.goto(`/t/${seedTask.id}`);
    await session.waitForLoad();
    await expect(notesEditor).toHaveCount(0);
    await expect(testPage.getByRole("button", { name: "Panels" })).toHaveCount(0);
    await expect(testPage.getByRole("button", { name: "Chat" })).toHaveClass(/text-primary/);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });

  test("passes mobile presentation to the kanban plugin action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await installFixturePlugin(testPage);

    const task = await apiClient.createTask(seedData.workspaceId, "Mobile plugin menu task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.taskCard(task.id).getByRole("button", { name: "More options" }).click();

    const menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible');
    await menu.getByRole("menuitem", { name: "Edit", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Enhance notes" }).click();

    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/plugins/${PLUGIN_ID}/user-state/task/${task.id}/menu-presentation`,
          );
          if (res.status !== 200) return null;
          const body = (await res.json()) as { value: string };
          return body.value;
        },
        { timeout: 10_000, intervals: [250, 500, 1000] },
      )
      .toBe("mobile");
  });
});
