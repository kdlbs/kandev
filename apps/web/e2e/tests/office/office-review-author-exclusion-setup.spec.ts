import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import { authorExclusionWorkflowYaml } from "../../helpers/author-exclusion";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

/**
 * REQ-OFFICE-REVIEW-AUTHOR-EXCL-005: a workflow that keeps reviewers off the
 * author's provider warns, without blocking, when the current setup would
 * not enforce it. The import dialog pauses on the warning, the import still
 * succeeds, and the provider routing page shows the same warning until the
 * setup is fixed.
 */
test.describe("Office reviewer exclusion setup warnings", () => {
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("import warns before importing; routing banner clears once two providers are set", async ({
    testPage,
    backend,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    test.setTimeout(120_000);
    const name = `Guarded review ${Date.now()}`;
    let workflowId: string | undefined;
    try {
      const settings = new WorkflowSettingsPage(testPage);
      await settings.goto(officeSeed.workspaceId);
      await testPage.getByRole("button", { name: "Import", exact: true }).click();
      const dialog = testPage.getByRole("dialog");
      await dialog.locator("textarea").fill(authorExclusionWorkflowYaml(name));
      await dialog.getByRole("button", { name: "Import", exact: true }).click();

      const review = testPage.getByTestId("workflow-import-profile-selection");
      await expect(review.getByText("Review imported workflows")).toBeVisible();
      const importWarnings = review.getByTestId("author-exclusion-warnings");
      await expect(importWarnings).toContainText(
        `${name} / Review (Reviewer): routing is off, so the exclusion is not enforced.`,
      );

      const imported = waitForHttp(
        testPage,
        "POST",
        new RegExp(`/api/v1/workspaces/${officeSeed.workspaceId}/workflows/import$`),
      );
      await review.getByTestId("workflow-import-profile-submit").click();
      expect((await imported).ok()).toBe(true);
      await expect(review).not.toBeVisible();
      const { workflows } = await apiClient.listWorkflows(officeSeed.workspaceId);
      workflowId = workflows.find((wf) => wf.name === name)?.id;
      expect(workflowId).toBeTruthy();
      // Import success refreshes the settings route; let it settle before navigating.
      await expect(await settings.findWorkflowCard(name, { waitForName: true })).toBeVisible();

      await testPage.goto("/office/workspace/routing");
      const banner = testPage.getByTestId("author-exclusion-warnings");
      await expect(banner).toContainText("routing is off, so the exclusion is not enforced.");

      await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp,codex-acp" });
      const routing = await balancedExecutionProfileRouting(
        apiClient,
        officeApi,
        officeSeed.workspaceId,
        ["claude-acp", "codex-acp"],
      );
      await officeApi.updateRouting(officeSeed.workspaceId, routing);
      await testPage.goto("/office/workspace/routing");
      await expect(
        testPage.getByRole("heading", { name: "Provider routing", exact: true }),
      ).toBeVisible();
      await expect(testPage.getByTestId("author-exclusion-warnings")).toHaveCount(0);
    } finally {
      if (workflowId) await apiClient.deleteWorkflow(workflowId);
    }
  });
});
