import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import { authorExclusionWorkflowYaml } from "../../helpers/author-exclusion";
import { balancedExecutionProfileRouting } from "../../helpers/office-routing";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

type ReviewStep = {
  name: string;
  events?: { on_enter?: Array<{ config?: Record<string, unknown> }> };
};

/**
 * REQ-OFFICE-REVIEW-AUTHOR-EXCL-005/006: a workflow that keeps reviewers off
 * the author's provider warns, without blocking, when the current setup would
 * not enforce it. The import dialog pauses on the warning, the import still
 * succeeds, and the provider routing page shows the same warning until the
 * setup is fixed: by turning the step's Review on a different provider switch
 * off, or by configuring two providers.
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
      await expect(importWarnings).toContainText("Automatic provider fallback is off");
      await expect(importWarnings).toContainText(`${name} workflow, Review step (Reviewer)`);
      await expect(importWarnings).toContainText(
        "Turn on Automatic provider fallback in Office > Routing, or turn off Review on a different provider for this step there.",
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
      await expect(banner).toContainText(
        "Turn on Automatic provider fallback, or turn off Review on a different provider for this step.",
      );

      const toggle = testPage.getByRole("switch", {
        name: `Review on a different provider for ${name} workflow, Review step (Reviewer)`,
      });
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      const saveBar = testPage.getByTestId("settings-floating-save");
      await expect(saveBar).toHaveCount(0);

      // An unsaved change shows the shared save bar; Reset restores the stored value.
      await toggle.click();
      await expect(saveBar).toBeVisible();
      await saveBar.getByRole("button", { name: "Reset" }).click();
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      await expect(saveBar).toHaveCount(0);

      await toggle.click();
      const savedOff = waitForHttp(testPage, "PUT", /\/routing\/author-exclusion$/);
      await saveBar.getByRole("button", { name: "Save changes" }).click();
      expect((await savedOff).ok()).toBe(true);
      await expect(banner).toHaveCount(0);
      await expect(toggle).toHaveAttribute("aria-checked", "false");
      const reviewConfig = async () => {
        const { steps } = (await apiClient.listWorkflowSteps(workflowId!)) as {
          steps: ReviewStep[];
        };
        return steps.find((s) => s.name === "Review")?.events?.on_enter?.[0]?.config ?? {};
      };
      expect(await reviewConfig()).toEqual({ role: "reviewer" });

      await toggle.click();
      const savedOn = waitForHttp(testPage, "PUT", /\/routing\/author-exclusion$/);
      await saveBar.getByRole("button", { name: "Save changes" }).click();
      expect((await savedOn).ok()).toBe(true);
      await expect(banner).toContainText(`${name} workflow, Review step (Reviewer)`);
      expect(await reviewConfig()).toEqual({ role: "reviewer", exclude_author_provider: true });

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
