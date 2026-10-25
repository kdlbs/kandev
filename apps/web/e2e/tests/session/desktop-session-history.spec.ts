import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("desktop: session history", () => {
  test("hides terminal Dockview helpers until history is explicitly shown", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Desktop session history", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const primary = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      sessionId: `desktop-history-primary-${task.id}`,
    });
    const helper = await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: seedData.agentProfileId,
      sessionId: `desktop-history-helper-${task.id}`,
      completedAt: "2026-09-01T00:00:00Z",
    });
    await apiClient.setPrimarySession(primary.session_id);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const layout = testPage.getByTestId("dockview-task-layout");
    const primaryTab = layout.getByTestId(`session-tab-${primary.session_id}`);
    const helperTab = layout.getByTestId(`session-tab-${helper.session_id}`);

    await expect(primaryTab).toBeVisible({ timeout: 15_000 });
    await expect(helperTab).toHaveCount(0);
    await prCapture.screenshot("dockview-session-history-hidden-desktop", {
      caption: "Full-task desktop Dockview with terminal helper sessions hidden by default",
    });

    const historyToggle = layout.getByTestId("dockview-session-history-toggle");
    await expect(historyToggle).toHaveAttribute("aria-label", "Show session history");
    await historyToggle.click();
    await expect(helperTab).toBeVisible({ timeout: 5_000 });
    await expect(primaryTab).toBeVisible();
    await prCapture.screenshot("dockview-session-history-visible-desktop", {
      caption:
        "Full-task desktop Dockview after the explicit history control reveals a terminal helper",
    });

    await expect(historyToggle).toHaveAttribute("aria-label", "Hide session history");
    await historyToggle.click();
    await expect(helperTab).toHaveCount(0);
    await expect(primaryTab).toBeVisible();
  });

  test("archives an open helper conversation and reopens its retained transcript", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Archived helper conversation", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const primary = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      sessionId: `archive-primary-${task.id}`,
    });
    const helper = await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: seedData.agentProfileId,
      sessionId: `archive-helper-${task.id}`,
      completedAt: "2026-09-01T00:00:00Z",
    });
    await apiClient.setPrimarySession(primary.session_id);
    await apiClient.seedSessionMessage(helper.session_id, {
      type: "message",
      content: "retained helper evidence",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const layout = testPage.getByTestId("dockview-task-layout");
    await layout.getByTestId("dockview-session-history-toggle").click();
    const helperTab = session.sessionTabBySessionId(helper.session_id);
    await expect(helperTab).toBeVisible();
    await helperTab.click();
    await expect(session.chat.getByText("retained helper evidence")).toBeVisible();

    await helperTab.click({ button: "right" });
    await session.contextMenuItem("Delete").click();
    const confirmation = testPage.getByTestId("session-delete-confirm-popover");
    await expect(confirmation).toContainText("Delete session?");
    await confirmation.getByTestId("session-delete-confirm").click();
    await expect(confirmation).not.toBeVisible();

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((item) => item.id === helper.session_id)?.archived_at;
      })
      .toBeTruthy();
    await expect(helperTab).toHaveCount(0);
    await session.addPanelButton().click();
    const reopen = session.sessionReopenItem(helper.session_id);
    await expect(reopen).toBeVisible();
    await reopen.click();
    await expect(helperTab).toBeVisible();
    await expect(session.chat.getByText("retained helper evidence")).toBeVisible();
    await expect(session.chat.getByTestId("archived-session-new-agent-button")).toBeVisible();
    await expect(session.recoveryResumeButton()).not.toBeVisible();
    await prCapture.screenshot("archived-helper-transcript-desktop", {
      caption: "Archived helper conversation remains readable after cleanup and history reopen",
    });
    await session.chat.getByTestId("archived-session-new-agent-button").click();
    await expect(session.newSessionDialog()).toBeVisible();
  });
});
