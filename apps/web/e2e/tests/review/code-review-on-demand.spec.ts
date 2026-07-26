import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const REVIEWED_FILE = "review-target.ts";
const SECOND_FILE = "review-second.ts";

/**
 * Points the review runner at the mock agent.
 *
 * The built-in `code-review` utility agent seeds disabled, which makes the
 * resolver fall through to the user's default agent/model pair — so setting that
 * pair is all the on-demand path needs. It also keeps the spec honest about the
 * documented precedence rather than force-enabling a builtin behind the scenes.
 */
async function configureReviewer(apiClient: {
  listInferenceAgents: () => Promise<{
    agents: Array<{ id: string; models: Array<{ id: string }> }>;
  }>;
  saveUserSettings: (settings: Record<string, unknown>) => Promise<void>;
}) {
  // The inference-agents endpoint is the right source: its `id` is the
  // registered agent-type id the review runner resolves against. An agent row
  // UUID from /api/v1/agents is rejected as "not inference-capable".
  const { agents } = await apiClient.listInferenceAgents();
  const agent = agents.find((candidate) => candidate.models.length > 0) ?? agents[0];
  if (!agent) throw new Error("code review e2e: no inference-capable agent was discovered");
  await apiClient.saveUserSettings({
    default_utility_agent_id: agent.id,
    default_utility_model: agent.models[0]?.id ?? "",
  });
}

test.describe("Native code review — on demand", () => {
  test.describe.configure({ timeout: 180_000 });

  test("reviews the working changes and renders anchored findings in the diff", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await configureReviewer(apiClient);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Native Code Review E2E",
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

    // Real working-tree changes for the reviewer to read.
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
    git.createFile(SECOND_FILE, "export const second = true;\n");

    const changesTab = testPage.getByTestId("dockview-tab-changes");
    await expect(changesTab).toBeVisible();
    await changesTab.click();
    await expect(testPage.getByTestId(`file-row-${REVIEWED_FILE}`)).toBeVisible({
      timeout: 30_000,
    });

    await testPage
      .getByTestId("changes-panel")
      .getByRole("button", { name: "Diff", exact: true })
      .click();
    await testPage.getByRole("button", { name: "Expand review" }).click();
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();

    await testPage.screenshot({
      path: "e2e-artifacts/review-01-before-run.png",
      fullPage: false,
    });

    // Start the review.
    const runButton = dialog.getByTestId("review-run-changes");
    await expect(runButton).toBeVisible();
    await runButton.click();

    // Findings land as inline annotations in the diff.
    const findingCard = dialog.getByTestId("review-finding-card").first();
    await expect(findingCard).toBeVisible({ timeout: 90_000 });
    await expect(dialog.getByTestId("review-finding-title").first()).toContainText(
      "Unchecked value can be nil",
    );
    await expect(dialog.getByTestId("review-finding-severity-blocker").first()).toBeVisible();
    await expect(dialog.getByTestId("review-open-count")).toContainText("finding");

    await testPage.screenshot({
      path: "e2e-artifacts/review-02-findings.png",
      fullPage: false,
    });

    // The findings count is a navigator: open it and jump straight to a finding
    // instead of scrolling the diff to hunt for it. The popover portals to the
    // document body, so it is scoped to the page rather than the dialog.
    await dialog.getByTestId("review-open-count").click();
    const navItem = testPage.getByTestId("review-finding-nav-item").first();
    await expect(navItem).toBeVisible();
    await expect(testPage.getByTestId("review-findings-popover")).toBeVisible();
    await testPage.screenshot({
      path: "e2e-artifacts/review-04-findings-navigator.png",
      fullPage: false,
    });
    await navItem.click();
    await expect(dialog.locator("[data-review-finding-id]").first()).toBeInViewport({
      timeout: 15_000,
    });
    await testPage.screenshot({
      path: "e2e-artifacts/review-05-jumped-to-finding.png",
      fullPage: false,
    });

    // A finding is advisory: resolving it is the human's call and it persists.
    await findingCard.getByTestId("review-finding-resolve").click();
    await expect(dialog.getByTestId("review-finding-card").first()).toHaveAttribute(
      "data-finding-status",
      "resolved",
      { timeout: 15_000 },
    );

    await testPage.screenshot({
      path: "e2e-artifacts/review-03-resolved.png",
      fullPage: false,
    });

    // Findings are backend-persisted, unlike pending inline comments. After a
    // reload the review dialog is closed and the Changes panel may be the active
    // tab, so wait on that tab rather than the chat panel.
    await testPage.reload();
    const changesTabAfterReload = testPage.getByTestId("dockview-tab-changes");
    await expect(changesTabAfterReload).toBeVisible({ timeout: 30_000 });
    await changesTabAfterReload.click();
    await testPage
      .getByTestId("changes-panel")
      .getByRole("button", { name: "Diff", exact: true })
      .click();
    await testPage.getByRole("button", { name: "Expand review" }).click();
    const reopened = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reopened).toBeVisible();
    await expect(reopened.getByTestId("review-finding-card").first()).toHaveAttribute(
      "data-finding-status",
      "resolved",
      { timeout: 30_000 },
    );
  });

  test("explains how to configure a reviewer when none is available", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // No default utility agent/model and a disabled builtin: the run must fail
    // closed with an actionable message rather than a generic error.
    await apiClient.saveUserSettings({
      default_utility_agent_id: "",
      default_utility_model: "",
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Native Code Review Unavailable E2E",
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

    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(REVIEWED_FILE, "export const unreviewable = 1;\n");

    await testPage.getByTestId("dockview-tab-changes").click();
    await expect(testPage.getByTestId(`file-row-${REVIEWED_FILE}`)).toBeVisible({
      timeout: 30_000,
    });
    await testPage
      .getByTestId("changes-panel")
      .getByRole("button", { name: "Diff", exact: true })
      .click();
    await testPage.getByRole("button", { name: "Expand review" }).click();
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();

    await dialog.getByTestId("review-run-changes").click();
    await expect(dialog.getByTestId("review-agent-unavailable")).toBeVisible({ timeout: 30_000 });
    await expect(dialog.getByTestId("review-finding-card")).toHaveCount(0);

    await testPage.screenshot({
      path: "e2e-artifacts/review-04-agent-unavailable.png",
      fullPage: false,
    });
  });
});
