import { test, expect } from "../../fixtures/office-fixture";
import { authorExclusionWorkflowYaml } from "../../helpers/author-exclusion";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Office reviewer exclusion setup warnings on mobile", () => {
  test("import drawer and routing banner show the warning without horizontal overflow", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const name = `Mobile guarded review ${Date.now()}`;
    let workflowId: string | undefined;
    try {
      const settings = new WorkflowSettingsPage(testPage);
      await settings.goto(officeSeed.workspaceId);
      await testPage.getByRole("button", { name: "Import", exact: true }).tap();
      const dialog = testPage.getByRole("dialog");
      await dialog.locator("textarea").fill(authorExclusionWorkflowYaml(name));
      await dialog.getByRole("button", { name: "Import", exact: true }).tap();

      const review = testPage.getByTestId("workflow-import-profile-selection");
      await expect(review.getByTestId("author-exclusion-warnings")).toContainText(
        `${name} workflow, Review step (Reviewer): Automatic provider fallback is off`,
      );
      await assertNoDocumentHorizontalOverflow(testPage, "workflow import exclusion warnings");
      await review.getByTestId("workflow-import-profile-submit").tap();
      await expect(review).not.toBeVisible();
      const { workflows } = await apiClient.listWorkflows(officeSeed.workspaceId);
      workflowId = workflows.find((wf) => wf.name === name)?.id;
      expect(workflowId).toBeTruthy();
      // Import success refreshes the settings route; let it settle before navigating.
      await expect(await settings.findWorkflowCard(name, { waitForName: true })).toBeVisible();

      await testPage.goto("/office/workspace/routing");
      const banner = testPage.getByTestId("author-exclusion-warnings");
      await expect(banner).toContainText(`${name} workflow, Review step (Reviewer)`);
      await assertNoDocumentHorizontalOverflow(testPage, "routing exclusion warning banner");
      const toggle = testPage.getByRole("switch", {
        name: `Review on a different provider for ${name} workflow, Review step (Reviewer)`,
      });
      await toggle.scrollIntoViewIfNeeded();
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      await assertNoDocumentHorizontalOverflow(testPage, "review on a different provider switches");
    } finally {
      if (workflowId) await apiClient.deleteWorkflow(workflowId);
    }
  });
});
