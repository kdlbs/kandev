import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test("explains unavailable workspace restore on desktop and phone", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile workspace recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const stamp = "workspace-unavailable-fixture";
  const occurredAt = new Date().toISOString();
  const message = "Worktree setup failed";
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "FAILED",
    completedAt: occurredAt,
    agentProfileId: seedData.agentProfileId,
    errorMessage: message,
    metadata: {
      last_agent_error: {
        message,
        stamp,
        scope: "session",
        phase: "bootstrap",
        code: "generic_launch_failure",
        occurred_at: occurredAt,
      },
    },
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "status",
    content: `Agent startup failed: ${message}`,
    createdAt: occurredAt,
    metadata: {
      recovery_actions: true,
      scope: "session",
      error_stamp: stamp,
      failure_kind: "generic_launch_failure",
      phase: "bootstrap",
      workspace_restorable: false,
      actions: [
        {
          type: "ws_request",
          label: "Start fresh session",
          params: {
            method: "session.recover",
            payload: { task_id: task.id, session_id: sessionId, action: "fresh_start" },
          },
        },
      ],
    },
  });

  await desktopTestPage.goto(`/t/${task.id}`);
  const desktopSession = new SessionPage(desktopTestPage);
  await desktopSession.waitForLoad();
  const desktopCard = desktopSession.activeChat().getByTestId("session-recovery-card");
  await expect(desktopCard).toBeVisible();
  await expect(desktopCard.getByTestId("session-recovery-prerequisite")).toHaveText(
    "No workspace was created, so there is nothing to restore.",
  );
  await expect(desktopCard.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);
  if (prCapture.capturing) {
    await desktopCard.scrollIntoViewIfNeeded();
    await prCapture.screenshot("desktop-workspace-unavailable-recovery", {
      page: desktopTestPage,
      caption: "Desktop recovery explains that no workspace exists and omits restore.",
      fullPage: true,
    });
  }

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const card = session.activeChat().getByTestId("session-recovery-card");
  await expect(card).toBeVisible();
  await expect(card.getByTestId("session-recovery-prerequisite")).toHaveText(
    "No workspace was created, so there is nothing to restore.",
  );
  await expect(card.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);

  const freshStart = card.getByTestId("recovery-fresh-button");
  await expect(freshStart).toBeVisible();
  const freshStartBox = await freshStart.boundingBox();
  expect(freshStartBox).not.toBeNull();
  expect(freshStartBox!.height).toBeGreaterThanOrEqual(44);
  await assertNoDocumentHorizontalOverflow(testPage, "workspace recovery on phone");
  if (prCapture.capturing) {
    await card.scrollIntoViewIfNeeded();
    await prCapture.screenshot("phone-workspace-unavailable-recovery", {
      caption: "Phone recovery explains that no workspace exists and omits restore.",
      fullPage: true,
    });
  }

  await testPage.reload();
  await session.waitForLoad();
  const reloadedCard = session.activeChat().getByTestId("session-recovery-card");
  await expect(reloadedCard.getByTestId("session-recovery-prerequisite")).toHaveText(
    "No workspace was created, so there is nothing to restore.",
  );
  await expect(reloadedCard.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage, "reloaded workspace recovery on phone");
});
