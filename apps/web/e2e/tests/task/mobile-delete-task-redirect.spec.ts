import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { holdMutationResponse } from "./removal-transition-helpers";

async function deleteFromMobileDrawer(
  page: Page,
  session: SessionPage,
  title: string,
): Promise<void> {
  await session.mobileSessionMenu.tap();
  const drawer = page.getByRole("dialog", { name: "Tasks" });
  const row = drawer.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(row).toBeVisible({ timeout: 15_000 });
  await row.getByRole("button", { name: "Task actions" }).tap();
  await page.getByRole("menuitem", { name: "Delete", exact: true }).tap();

  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  const discard = dialog.getByTestId("delete-discard-worktree-checkbox");
  if (await discard.isVisible()) await discard.tap();
  await dialog.getByRole("button", { name: "Delete", exact: true }).tap();
}

test.describe("Mobile delete task redirect", () => {
  test("closes the drawer and protects the last task while delete settles", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const first = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Delete First",
      seedData.agentProfileId,
      {
        description: 'e2e:message("mobile delete first response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const last = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Delete Last",
      seedData.agentProfileId,
      {
        description: 'e2e:message("mobile delete last response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${first.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

    await deleteFromMobileDrawer(testPage, session, "Mobile Delete First");
    await expect(testPage).toHaveURL(new RegExp(`/t/${last.id}$`), { timeout: 20_000 });
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

    const documentRequests: string[] = [];
    testPage.on("request", (request) => {
      if (
        request.isNavigationRequest() &&
        request.frame() === testPage.mainFrame() &&
        request.resourceType() === "document"
      ) {
        documentRequests.push(request.url());
      }
    });
    const gate = await holdMutationResponse(testPage, `**/api/v1/tasks/${last.id}*`, "DELETE");

    try {
      await deleteFromMobileDrawer(testPage, session, "Mobile Delete Last");
      await gate.backendResponseReady;

      await expect(testPage.getByRole("dialog", { name: "Tasks" })).toHaveCount(0);
      const status = testPage.getByTestId("task-removal-status");
      await expect(status).toBeVisible({ timeout: 10_000 });
      await expect(session.activeChat()).toHaveCount(0);
      await expect
        .poll(() => testPage.evaluate(() => document.activeElement?.textContent ?? ""))
        .toContain("Leaving this task");
      expect(gate.requestCount()).toBe(1);
      expect(documentRequests).toHaveLength(0);

      gate.release();
      await expect(testPage).not.toHaveURL(/\/t\//, { timeout: 20_000 });
      expect(documentRequests).toHaveLength(0);
    } finally {
      gate.release();
      await gate.dispose();
    }
  });
});
