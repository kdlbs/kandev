import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import {
  assertBaseCommitReachable,
  prepareLocalBaseScenario,
  restoreLocalBaseRepository,
  waitForGitSuccess,
} from "./local-base-operations-helpers";

test.describe("Mobile local-only Git operations", () => {
  test.setTimeout(120_000);

  test.afterEach(({ backend, seedData }) => {
    restoreLocalBaseRepository(seedData, backend);
  });

  test("rebases a local base without origin", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const scenario = prepareLocalBaseScenario(seedData, backend, "rebase");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile local-only rebase",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [
          {
            repository_id: seedData.repositoryId,
            base_branch: "main",
            checkout_branch: scenario.branch,
          },
        ],
      },
    );

    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 45_000 });

    const gitActions = testPage.getByTestId("mobile-git-actions");
    await expect(gitActions).toBeVisible();
    await gitActions.tap();
    const menu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expect(menu).toBeVisible();
    await menu
      .locator('[data-slot="dropdown-menu-item"]')
      .filter({ hasText: /^Rebase/ })
      .tap();

    await waitForGitSuccess(testPage, "Rebase");
    assertBaseCommitReachable(scenario.git, scenario.baseHead);
  });
});
