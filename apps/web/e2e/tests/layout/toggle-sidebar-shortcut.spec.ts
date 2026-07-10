import { test, expect } from "../../fixtures/test-base";
import type { Page, Locator } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// The TOGGLE_SIDEBAR shortcut lives on a global app-root listener (useAppShortcuts),
// so Cmd/Ctrl+B must collapse/expand the unified AppSidebar on every route — not
// just inside the dockview session editor. It is UNBOUND by default on this
// branch, so each test binds it to Cmd/Ctrl+B first (mirroring how a user would
// set it in Settings).

const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";

/** Bind TOGGLE_SIDEBAR to Cmd/Ctrl+B (unbound by default) for the active user. */
async function bindToggleSidebar(apiClient: ApiClient, seedData: SeedData): Promise<void> {
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    keyboard_shortcuts: {
      TOGGLE_SIDEBAR: { key: "b", modifiers: { ctrlOrCmd: true } },
    },
  });
}

/** Press Cmd/Ctrl+B and assert the AppSidebar's collapsed state flips both ways. */
async function expectShortcutTogglesSidebar(page: Page, sidebar: Locator): Promise<void> {
  await expect(sidebar).toBeVisible();
  // The shortcut is intentionally ignored while typing — clear focus first so a
  // page-autofocused input can't suppress it.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());

  const initial = (await sidebar.getAttribute("data-collapsed")) ?? "false";
  const flipped = initial === "true" ? "false" : "true";

  await page.keyboard.press(`${MODIFIER}+b`);
  await expect(sidebar).toHaveAttribute("data-collapsed", flipped);

  await page.keyboard.press(`${MODIFIER}+b`);
  await expect(sidebar).toHaveAttribute("data-collapsed", initial);
}

test.describe("Toggle sidebar shortcut (global)", () => {
  test("toggles the AppSidebar on the Kanban board", async ({ testPage, apiClient, seedData }) => {
    await bindToggleSidebar(apiClient, seedData);
    await testPage.goto("/");
    await expectShortcutTogglesSidebar(testPage, testPage.getByTestId("app-sidebar"));
  });

  test("toggles the AppSidebar on a Settings page", async ({ testPage, apiClient, seedData }) => {
    await bindToggleSidebar(apiClient, seedData);
    await testPage.goto("/settings/general");
    await expectShortcutTogglesSidebar(testPage, testPage.getByTestId("app-sidebar"));
  });

  test("still toggles the AppSidebar on the dockview session page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await bindToggleSidebar(apiClient, seedData);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Toggle sidebar shortcut session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Target the global rail (app-sidebar), not the dockview's per-session
    // sidebar (task-sidebar).
    await expectShortcutTogglesSidebar(testPage, testPage.getByTestId("app-sidebar"));
  });
});
