import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

/**
 * Seed a task + session and navigate to the session page.
 * Waits for the mock agent to finish (idle input visible).
 */
async function seedAndNavigate(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Toolbar Overflow Test",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/s/${task.session_id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
}

/** Force the toolbar container to a specific max-width via inline style. */
async function constrainToolbar(page: Page, maxWidth: string | null) {
  await page.evaluate((mw) => {
    const el = document.querySelector('[data-testid="chat-input-toolbar"]') as HTMLElement;
    if (!el) return;
    if (mw) {
      el.style.maxWidth = mw;
    } else {
      el.style.removeProperty("max-width");
    }
  }, maxWidth);
  // Allow ResizeObserver to fire
  await page.waitForTimeout(200);
}

test.describe("Toolbar overflow menu", () => {
  test.describe.configure({ retries: 1 });

  test("collapses toolbar items and expands inline when toggled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedAndNavigate(testPage, apiClient, seedData);

    const toolbar = testPage.getByTestId("chat-input-toolbar");
    await expect(toolbar).toBeVisible({ timeout: 10_000 });

    // At default width, collapsible items should be visible inline
    const modelItem = testPage.getByTestId("toolbar-item-model");
    const mcpItem = testPage.getByTestId("toolbar-item-mcp");
    const overflowBtn = testPage.getByTestId("toolbar-overflow-menu");

    await expect(modelItem).toBeVisible({ timeout: 5_000 });
    await expect(mcpItem).toBeVisible({ timeout: 5_000 });
    await expect(overflowBtn).not.toBeVisible();

    // Constrain toolbar to a narrow width to force overflow
    await constrainToolbar(testPage, "300px");

    // Collapsible items should disappear and expand toggle should appear
    await expect(overflowBtn).toBeVisible({ timeout: 5_000 });
    await expect(modelItem).not.toBeVisible();

    // Submit button should remain visible (always-visible item)
    const submitBtn = toolbar.locator("button.rounded-full");
    await expect(submitBtn).toBeVisible();

    // Click expand toggle — items appear inline (scrollable)
    await overflowBtn.click();
    await expect(modelItem).toBeVisible({ timeout: 5_000 });
    await expect(mcpItem).toBeVisible({ timeout: 5_000 });

    // Collapse button should still be visible to toggle back
    await expect(overflowBtn).toBeVisible();

    // Click again to collapse back
    await overflowBtn.click();
    await expect(modelItem).not.toBeVisible({ timeout: 5_000 });

    // Remove constraint — items should reappear inline normally
    await constrainToolbar(testPage, null);

    await expect(modelItem).toBeVisible({ timeout: 5_000 });
    await expect(mcpItem).toBeVisible({ timeout: 5_000 });
    await expect(overflowBtn).not.toBeVisible();
  });
});
