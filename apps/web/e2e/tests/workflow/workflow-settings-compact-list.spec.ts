import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

const RENAMED = "Compact Rename Target Renamed";

test.describe("Workflow settings compact list", () => {
  test("defaults to compact rows that expand, keep drafts when collapsed, and persist the view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Rename a workflow owned by this test: the seeded workflow is shared by
    // later specs that look it up by name.
    const target = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Compact Rename Target",
      "simple",
    );
    const { steps } = await apiClient.listWorkflowSteps(target.id);
    const stepNames = [...steps].sort((a, b) => a.position - b.position).map((s) => s.name);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId, { listView: null });

    await expect(page.listViewOption("compact")).toHaveAttribute("data-state", "on");
    const card = page.workflowCard(target.id);
    const row = page.compactRow(target.id);
    await expect(row).toBeVisible();
    await expect(row).toHaveAttribute("aria-expanded", "false");
    await expect(card).toHaveAttribute("data-expanded", "false");
    await expect(row.getByTestId("workflow-compact-step")).toHaveText(stepNames);
    await expect(row).toContainText(
      `${stepNames.length} ${stepNames.length === 1 ? "step" : "steps"}`,
    );
    await expect(page.deleteWorkflowButton(card)).toBeHidden();

    await row.click();
    await expect(card).toHaveAttribute("data-expanded", "true");
    const nameInput = card.locator("input").first();
    await expect(nameInput).toBeVisible();
    await nameInput.fill(RENAMED);

    await row.click();
    await expect(card).toHaveAttribute("data-expanded", "false");
    await expect(nameInput).toBeHidden();
    await expect(row).toContainText(RENAMED);
    await expect(row).toContainText("Unsaved");
    await expect(page.floatingSave).toBeVisible();

    await row.click();
    await expect(nameInput).toHaveValue(RENAMED);
    await page.saveChanges();
    await expect
      .poll(async () => {
        const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
        return workflows.find((wf) => wf.id === target.id)?.name;
      })
      .toBe(RENAMED);
    await expect(row).not.toContainText("Unsaved");

    await page.listViewOption("detailed").click();
    await expect(row).toHaveCount(0);
    await expect(nameInput).toBeVisible();

    await testPage.reload();
    await expect(page.listViewOption("detailed")).toHaveAttribute("data-state", "on");
    await expect(page.compactRow(target.id)).toHaveCount(0);
    await expect(page.workflowCard(target.id).locator("input").first()).toHaveValue(RENAMED);
  });

  test("reorders workflows by dragging collapsed rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const second = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Second Workflow",
      "simple",
    );
    await apiClient.reorderWorkflows(seedData.workspaceId, [seedData.workflowId, second.id]);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId, { listView: null });

    const cards = testPage.locator('[data-testid^="workflow-card-"]');
    await expect(cards).toHaveCount(2);
    await expect(cards.first()).toHaveAttribute(
      "data-testid",
      `workflow-card-${seedData.workflowId}`,
    );

    const handle = page.dragHandle(second.id);
    const firstRow = page.compactRow(seedData.workflowId);
    const [handleBox, rowBox] = await Promise.all([handle.boundingBox(), firstRow.boundingBox()]);
    expect(handleBox).not.toBeNull();
    expect(rowBox).not.toBeNull();
    // The handle sits inside its own collapsed row.
    const secondRowBox = await page.compactRow(second.id).boundingBox();
    expect(handleBox!.y).toBeGreaterThanOrEqual(secondRowBox!.y - 1);
    expect(handleBox!.y + handleBox!.height).toBeLessThanOrEqual(
      secondRowBox!.y + secondRowBox!.height + 1,
    );

    await testPage.mouse.move(
      handleBox!.x + handleBox!.width / 2,
      handleBox!.y + handleBox!.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(handleBox!.x + handleBox!.width / 2, rowBox!.y + 2, { steps: 12 });
    await testPage.mouse.up();

    await expect(cards.first()).toHaveAttribute("data-testid", `workflow-card-${second.id}`);
    await expect(page.floatingSave).toBeVisible();
  });
});
