// Filename starts with "mobile-" so the mobile-chrome project exercises the
// touch path for the review findings navigator (the new "N findings" control).
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const REVIEWED_FILE = "review-target.ts";

/** Points the review runner at the mock agent via the user's default pair. */
async function configureReviewer(apiClient: {
  listInferenceAgents: () => Promise<{
    agents: Array<{ id: string; models: Array<{ id: string }> }>;
  }>;
  saveUserSettings: (settings: Record<string, unknown>) => Promise<void>;
}) {
  const { agents } = await apiClient.listInferenceAgents();
  const agent = agents.find((candidate) => candidate.models.length > 0) ?? agents[0];
  if (!agent) throw new Error("mobile review nav e2e: no inference-capable agent was discovered");
  await apiClient.saveUserSettings({
    default_utility_agent_id: agent.id,
    default_utility_model: agent.models[0]?.id ?? "",
  });
}

test.describe("Review findings navigator on mobile", () => {
  test.describe.configure({ retries: 2, timeout: 180_000 });

  test("tapping the findings count jumps to a finding", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await configureReviewer(apiClient);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Review Findings Nav Mobile E2E",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();

    // Real working-tree changes for the reviewer to anchor findings to.
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(REVIEWED_FILE, "export const existing = 1;\n");
    git.stageAll();
    git.commit("seed review target");
    git.modifyFile(
      REVIEWED_FILE,
      "export const existing = 1;\nexport const added = value;\nexport const other = 2;\n",
    );

    // Open the changes panel via the mobile nav, then the review dialog.
    const mobileChangesButton = testPage.getByRole("button", { name: /Changes/ }).last();
    await expect(mobileChangesButton).toBeVisible({ timeout: 15_000 });
    await mobileChangesButton.tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId(`file-row-${REVIEWED_FILE}`)).toBeVisible({
      timeout: 30_000,
    });
    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible({ timeout: 10_000 });

    // Run the review; findings render inline once it completes.
    await dialog.getByTestId("review-run-changes").tap();
    await expect(dialog.getByTestId("review-finding-card").first()).toBeVisible({
      timeout: 90_000,
    });

    // The findings count is the touch navigator: tap it, tap a finding, and the
    // target card is scrolled into view.
    const findingsCount = dialog.getByTestId("review-open-count");
    await expect(findingsCount).toContainText("finding");
    await findingsCount.tap();
    const navItem = testPage.getByTestId("review-finding-nav-item").first();
    await expect(navItem).toBeVisible();
    await navItem.tap();
    await expect(dialog.locator("[data-review-finding-id]").first()).toBeInViewport({
      timeout: 15_000,
    });
  });
});
