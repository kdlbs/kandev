import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  attachSessionLaunchCapture,
  countPeerDeliveries,
  createWorkflowPeerResumeScenario,
  waitForPeerSessionState,
  type WorkflowPeerResumeScenario,
} from "./workflow-peer-resume-helpers";

test.describe("mobile: workflow peer resume", () => {
  test("mobile review peer message resumes implement without parking UI", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    let scenario: WorkflowPeerResumeScenario | undefined;

    try {
      scenario = await createWorkflowPeerResumeScenario(apiClient, seedData, "Mobile peer resume");
      const beforeMessage = await apiClient.getTask(scenario.taskId);
      expect(beforeMessage.workflow_step_id).toBe(scenario.reviewStepId);
      expect(beforeMessage.primary_session_id).toBe(scenario.initialSessionId);

      const sessionsBefore = await apiClient.listTaskSessions(scenario.taskId);
      const implementationBefore = sessionsBefore.sessions.find(
        (session) => session.id === scenario!.implementationSessionId,
      );
      expect(implementationBefore?.metadata?.workflow_parking).toBeDefined();

      await waitForPeerSessionState(
        apiClient,
        scenario.taskId,
        scenario.implementationSessionId,
        "RUNNING",
      );

      const sessionPage = new SessionPage(testPage);
      await testPage.goto(`/t/${scenario.taskId}`);
      await sessionPage.waitForLoad();
      const mobileLayout = testPage.locator('[data-testid="mobile-task-layout"]:visible');
      await mobileLayout.getByTestId("mobile-sessions-pill").tap();
      const sessionPicker = testPage.getByRole("dialog", { name: "Sessions" });
      const implementationRow = sessionPicker.getByTestId(
        `mobile-session-row-${scenario.implementationSessionId}`,
      );
      await expect(implementationRow).toBeVisible();
      const implementationRowBox = await implementationRow.boundingBox();
      expect(implementationRowBox).not.toBeNull();
      expect(implementationRowBox!.height).toBeGreaterThanOrEqual(44);
      await implementationRow.tap();
      await expect(sessionPicker).not.toBeVisible();
      await sessionPage.expectChatResponseVisible(scenario.outputMarker);
      await expect(testPage.locator('[data-testid="task-parked-session-note"]')).toHaveCount(0);
      await expect(
        sessionPage.activeChat().locator(".tiptap.ProseMirror:visible").first(),
      ).toBeEditable();
      await assertNoDocumentHorizontalOverflow(testPage, "active peer-resumed mobile session");
      await expect
        .poll(() =>
          countPeerDeliveries(apiClient, scenario!.implementationSessionId, scenario!.outputMarker),
        )
        .toBe(1);

      const activeTask = await apiClient.getTask(scenario.taskId);
      expect(activeTask.workflow_step_id).toBe(scenario.reviewStepId);
      expect(activeTask.primary_session_id).toBe(scenario.initialSessionId);
      const activeSessions = await apiClient.listTaskSessions(scenario.taskId);
      expect(
        activeSessions.sessions.find((session) => session.id === scenario!.implementationSessionId)
          ?.metadata?.workflow_parking,
      ).toBeDefined();

      await testPage.reload();
      await sessionPage.waitForLoad();
      const reloadedMobileLayout = testPage.locator('[data-testid="mobile-task-layout"]:visible');
      await reloadedMobileLayout.getByTestId("mobile-sessions-pill").tap();
      const reloadedSessionPicker = testPage.getByRole("dialog", { name: "Sessions" });
      await reloadedSessionPicker
        .getByTestId(`mobile-session-row-${scenario.implementationSessionId}`)
        .tap();
      await expect(reloadedSessionPicker).not.toBeVisible();
      await expect(testPage.locator('[data-testid="task-parked-session-note"]')).toHaveCount(0);
      await sessionPage.expectChatResponseVisible(scenario.outputMarker);
      await assertNoDocumentHorizontalOverflow(
        testPage,
        "reloaded active peer-resumed mobile session",
      );

      await waitForPeerSessionState(
        apiClient,
        scenario.taskId,
        scenario.implementationSessionId,
        "WAITING_FOR_INPUT",
      );
      await sessionPage.expectChatResponseVisible(scenario.completionMarker);
      await expect(testPage.locator('[data-testid="task-parked-session-note"]')).toHaveCount(0);
      const finalTask = await apiClient.getTask(scenario.taskId);
      expect(finalTask.workflow_step_id).toBe(scenario.reviewStepId);
      expect(finalTask.primary_session_id).toBe(scenario.initialSessionId);
      await expect
        .poll(() =>
          countPeerDeliveries(apiClient, scenario!.implementationSessionId, scenario!.outputMarker),
        )
        .toBe(1);
    } finally {
      if (scenario) {
        await Promise.all(
          [scenario.implementationSessionId, scenario.initialSessionId].map((sessionId) =>
            apiClient
              .stopSession({
                session_id: sessionId,
                reason: "mobile workflow peer resume e2e cleanup",
                force: true,
              })
              .catch(() => undefined),
          ),
        );
      }
    }
  });

  test("mobile opens parked implement and resumes without changing Review ownership", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    let scenario: WorkflowPeerResumeScenario | undefined;
    const capture = attachSessionLaunchCapture(testPage);

    try {
      scenario = await createWorkflowPeerResumeScenario(apiClient, seedData, "Mobile open resume", {
        sendPeerMessage: false,
      });
      expect(
        await countPeerDeliveries(
          apiClient,
          scenario.implementationSessionId,
          scenario.implementationMarker,
        ),
      ).toBe(1);
      const beforeOpen = await apiClient.listTaskSessions(scenario.taskId);
      expect(
        beforeOpen.sessions.find((session) => session.id === scenario!.implementationSessionId)
          ?.metadata?.workflow_parking,
      ).toBeDefined();

      const sessionPage = new SessionPage(testPage);
      await testPage.goto(`/t/${scenario.taskId}`);
      await sessionPage.waitForLoad();
      const mobileLayout = testPage.locator('[data-testid="mobile-task-layout"]:visible');
      await mobileLayout.getByTestId("mobile-sessions-pill").tap();
      const sessionPicker = testPage.getByRole("dialog", { name: "Sessions" });
      const implementationRow = sessionPicker.getByTestId(
        `mobile-session-row-${scenario.implementationSessionId}`,
      );
      await expect(implementationRow).toBeVisible();
      const implementationRowBox = await implementationRow.boundingBox();
      expect(implementationRowBox).not.toBeNull();
      expect(implementationRowBox!.height).toBeGreaterThanOrEqual(44);
      await implementationRow.tap();
      await expect(sessionPicker).not.toBeVisible();
      await expect
        .poll(() =>
          capture.requests.some(
            (request) =>
              request.taskId === scenario!.taskId &&
              request.sessionId === scenario!.implementationSessionId &&
              request.intent === "resume" &&
              request.activationSource === "session_open",
          ),
        )
        .toBe(true);
      await expect(
        sessionPage.activeChat().getByText("Resumed agent", { exact: false }).first(),
      ).toBeVisible({ timeout: 30_000 });
      await expect(testPage.locator('[data-testid="task-parked-session-note"]')).toHaveCount(0);
      await expect(
        sessionPage.activeChat().locator(".tiptap.ProseMirror:visible").first(),
      ).toBeEditable();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile session-open recovery");
      await expect
        .poll(() =>
          countPeerDeliveries(
            apiClient,
            scenario!.implementationSessionId,
            scenario!.implementationMarker,
          ),
        )
        .toBe(1);

      const task = await apiClient.getTask(scenario.taskId);
      expect(task.workflow_step_id).toBe(scenario.reviewStepId);
      expect(task.primary_session_id).toBe(scenario.initialSessionId);
    } finally {
      if (scenario) {
        await Promise.all(
          [scenario.implementationSessionId, scenario.initialSessionId].map((sessionId) =>
            apiClient
              .stopSession({
                session_id: sessionId,
                reason: "mobile workflow session-open recovery e2e cleanup",
                force: true,
              })
              .catch(() => undefined),
          ),
        );
      }
    }
  });
});
