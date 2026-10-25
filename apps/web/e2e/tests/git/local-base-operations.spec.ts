import { test } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import {
  assertBaseCommitReachable,
  openDesktopVcsMenu,
  prepareLocalBaseScenario,
  restoreLocalBaseRepository,
  waitForGitSuccess,
} from "./local-base-operations-helpers";

test.describe("Desktop local-only Git operations", () => {
  test.setTimeout(120_000);

  test.afterEach(({ backend, seedData }) => {
    restoreLocalBaseRepository(seedData, backend);
  });

  test("merges a local base without origin", async ({ testPage, apiClient, seedData, backend }) => {
    const scenario = prepareLocalBaseScenario(seedData, backend, "merge");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Desktop local-only merge",
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
    const menu = await openDesktopVcsMenu(session, testPage);
    await menu
      .locator('[data-slot="dropdown-menu-item"]')
      .filter({ hasText: /^Merge/ })
      .click();

    await waitForGitSuccess(testPage, "Merge");
    assertBaseCommitReachable(scenario.git, scenario.baseHead);
  });
});
