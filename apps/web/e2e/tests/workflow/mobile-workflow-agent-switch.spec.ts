import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  createWorkflowAgentProfiles,
  pollWorkflowTaskSessions,
  waitForWorkflowProfileSession,
} from "./workflow-agent-switch-helpers";

test.describe("Workflow agent profile switching on mobile", () => {
  test("shows parked and newly created workflow sessions in the task session picker", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { profileA, profileB } = await createWorkflowAgentProfiles(apiClient);
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile workflow session tabs",
    );
    const stepA = await apiClient.createWorkflowStep(workflow.id, "Profile A", 0, {
      is_start_step: true,
    });
    const stepB = await apiClient.createWorkflowStep(workflow.id, "Profile B", 1);
    const stepAAgain = await apiClient.createWorkflowStep(workflow.id, "Profile A Again", 2);

    await apiClient.updateWorkflowStep(stepA.id, {
      agent_profile_id: profileA.id,
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(stepB.id, {
      agent_profile_id: profileB.id,
      profile_session_start_policy: "reuse",
      profile_session_end_policy: "park",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.updateWorkflowStep(stepAAgain.id, {
      agent_profile_id: profileA.id,
      profile_session_start_policy: "new",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile workflow session tabs task",
      profileA.id,
      {
        workflow_id: workflow.id,
        workflow_step_id: stepA.id,
        repository_ids: [seedData.repositoryId],
      },
    );
    const originalASessionId = await waitForWorkflowProfileSession(apiClient, task.id, profileA.id);

    await apiClient.moveTask(task.id, workflow.id, stepB.id);
    await waitForWorkflowProfileSession(apiClient, task.id, profileB.id);
    await apiClient.moveTask(task.id, workflow.id, stepAAgain.id);

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.some(
            (session) =>
              session.agent_profile_id === profileA.id &&
              session.id !== originalASessionId &&
              session.is_primary &&
              session.state === "WAITING_FOR_INPUT",
          );
        },
        { timeout: 30_000, message: "new Profile A session never became primary and answerable" },
      )
      .toBe(true);
    const sessions = await pollWorkflowTaskSessions(apiClient, task.id, 3);
    const activeASession = sessions.find(
      (session) => session.agent_profile_id === profileA.id && session.is_primary,
    );
    const profileBSession = sessions.find((session) => session.agent_profile_id === profileB.id);
    const originalASession = sessions.find((session) => session.id === originalASessionId);
    expect(sessions).toHaveLength(3);
    expect(activeASession).toBeDefined();
    expect(originalASession).toMatchObject({
      agent_profile_id: profileA.id,
      is_primary: false,
      state: "WAITING_FOR_INPUT",
    });
    expect(profileBSession).toMatchObject({
      is_primary: false,
      state: "WAITING_FOR_INPUT",
    });

    await testPage.goto(`/t/${task.id}`);
    const details = new SessionPage(testPage);
    await details.waitForLoad();
    const picker = testPage.getByTestId("mobile-sessions-pill");
    await picker.tap();

    for (const taskSession of sessions) {
      const row = testPage.getByTestId(`mobile-session-row-${taskSession.id}`);
      await expect(row).toBeVisible();
      await expect(row).toContainText(
        taskSession.agent_profile_id === profileA.id ? profileA.name : profileB.name,
      );
      await expect(testPage.getByTestId(`mobile-session-state-${taskSession.id}`)).toBeVisible();
      const rowBox = await row.boundingBox();
      expect(rowBox).not.toBeNull();
      expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    }
    const activeRow = testPage.getByTestId(`mobile-session-row-${activeASession!.id}`);
    await expect(activeRow.locator(".tabler-icon-star")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "workflow session picker");

    await testPage.getByTestId(`mobile-session-row-${originalASessionId}`).tap();
    await picker.tap();
    await expect(testPage.getByTestId(`mobile-session-row-${originalASessionId}`)).toHaveAttribute(
      "aria-current",
      "true",
    );
  });
});
