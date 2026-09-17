import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createQueuedSessionOwnershipScenario,
  expectSessionMessagesUnchanged,
  QUEUED_DESTINATION_MARKER,
  sessionMessageIds,
  waitForLaunchQueueCleared,
  waitForSessionState,
  waitForTaskState,
} from "./queued-session-ownership-helpers";

test.describe("Queued session ownership", () => {
  test("keeps passive desktop inspection on the parked session and dispatches the queued destination once", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const releaseCeiling = await backend.useEnv({ KANDEV_MAX_CONCURRENT_SESSIONS: "1" });
    let fillerSessionId: string | undefined;

    try {
      const scenario = await createQueuedSessionOwnershipScenario(
        apiClient,
        seedData,
        "Desktop queued ownership",
      );
      fillerSessionId = scenario.fillerSessionId;

      const sourceMessagesBefore = await sessionMessageIds(apiClient, scenario.sourceSessionId);

      await testPage.goto(`/t/${scenario.taskId}`);
      const sessionPage = new SessionPage(testPage);
      await sessionPage.waitForLoad();

      const queueStatus = testPage.locator('[data-testid="task-launch-queue-status"]:visible');
      await expect(queueStatus).toBeVisible();
      await expect(queueStatus).toContainText("Queued");
      await expect(queueStatus).toContainText("Waiting for session capacity");
      await expect(queueStatus).toContainText("1 of 1");
      await expect(queueStatus).toContainText(scenario.destinationProfileName);
      await expect(
        sessionPage.sidebarTaskItem(scenario.taskTitle).getByTestId("sidebar-task-launch-queue"),
      ).toBeVisible();

      await sessionPage.sessionTabBySessionId(scenario.sourceSessionId).click();
      await expect(queueStatus).toBeVisible();
      await expect(
        testPage.locator('[data-testid="task-parked-session-note"]:visible'),
      ).toBeVisible();
      await expectSessionMessagesUnchanged(
        apiClient,
        scenario.sourceSessionId,
        sourceMessagesBefore,
      );

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(scenario.taskId);
            return sessions.find((session) => session.id === scenario.destinationSessionId)?.state;
          },
          { timeout: 5_000, message: "passive inspection changed the queued destination" },
        )
        .toBe("CREATED");
      await waitForTaskState(apiClient, scenario.taskId, "SCHEDULING", 5_000);

      await apiClient.stopSession({
        session_id: scenario.fillerSessionId,
        reason: "queued-session-ownership e2e capacity release",
        force: true,
      });
      fillerSessionId = undefined;
      await waitForLaunchQueueCleared(apiClient, seedData.workspaceId, scenario.taskId);
      await waitForSessionState(
        apiClient,
        scenario.taskId,
        scenario.destinationSessionId,
        "WAITING_FOR_INPUT",
        60_000,
      );
      await expectSessionMessagesUnchanged(
        apiClient,
        scenario.sourceSessionId,
        sourceMessagesBefore,
      );

      const destinationMessages = (
        await apiClient.listSessionMessages(scenario.destinationSessionId)
      ).messages;
      const workflowDeliveries = destinationMessages.filter(
        (message) =>
          message.author_type === "user" &&
          message.metadata?.workflow_message === true &&
          message.content.includes(QUEUED_DESTINATION_MARKER),
      );
      expect(workflowDeliveries).toHaveLength(1);
      expect(
        destinationMessages.some((message) => message.content.includes(QUEUED_DESTINATION_MARKER)),
      ).toBe(true);
    } finally {
      if (fillerSessionId) {
        await apiClient
          .stopSession({
            session_id: fillerSessionId,
            reason: "queued-session-ownership e2e cleanup",
            force: true,
          })
          .catch(() => undefined);
      }
      await releaseCeiling();
    }
  });
});
