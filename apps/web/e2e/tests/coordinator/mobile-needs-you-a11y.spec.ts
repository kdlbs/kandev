// AC-COORDINATOR-NEEDS-YOU-008.2: an automated accessibility scan of each
// coordinator screen shall report no critical violation
// (docs/specs/coordinator/requirements/needs-you.md, #phone-and-accessibility).
// Kept separate from mobile-needs-you.spec.ts so this file, and its
// @axe-core/playwright devDependency, can be dropped independently.
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { linkToCoordinatorNeedsYou, linkToCoordinatorQueue } from "../../../lib/coordinator/links";
import { runAxeScan, summarizeAxeViolations, type AxeScanResults } from "../../helpers/axe";

function criticalViolations(results: AxeScanResults): AxeScanResults["violations"] {
  return results.violations.filter((violation) => violation.impact === "critical");
}

test.describe("Coordinator screens on a phone viewport", () => {
  test("Needs you and Queue report no critical accessibility violation (AC .008.2)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Axe Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Axe Fixture",
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the clarification task");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should block before the Needs you screen is opened",
      timeout: 60_000,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    await expect(testPage.getByTestId(`needs-you-item-${task.id}`)).toBeVisible();
    const needsYouResults = await runAxeScan(testPage);
    const needsYouCritical = criticalViolations(needsYouResults);
    expect(
      needsYouCritical,
      summarizeAxeViolations({ ...needsYouResults, violations: needsYouCritical }),
    ).toEqual([]);

    await testPage.goto(linkToCoordinatorQueue(seedData.workspaceId, coordinator.id));
    await expect(testPage.getByTestId("coordinator-count-strip")).toBeVisible();
    const queueResults = await runAxeScan(testPage);
    const queueCritical = criticalViolations(queueResults);
    expect(
      queueCritical,
      summarizeAxeViolations({ ...queueResults, violations: queueCritical }),
    ).toEqual([]);
  });
});
