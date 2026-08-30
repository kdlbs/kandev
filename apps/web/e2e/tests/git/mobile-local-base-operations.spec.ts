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

  // Existing Changes behavior now documents @covers AC-UI-MOBILE-TASK-CHROME-001.4.
  test("rebases a local base without origin from Changes", async ({
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

    await testPage.getByRole("button", { name: "Changes" }).tap();
    const changes = testPage.getByTestId("mobile-changes-panel");
    const pullMenuTrigger = changes.getByRole("button", { name: /^Pull/ });
    await expect(pullMenuTrigger).toBeVisible();
    await pullMenuTrigger.tap();
    const menu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expect(menu).toBeVisible();
    await menu.getByRole("menuitem", { name: "Rebase", exact: true }).tap();

    await waitForGitSuccess(testPage, "Rebase");
    assertBaseCommitReachable(scenario.git, scenario.baseHead);
  });
});
