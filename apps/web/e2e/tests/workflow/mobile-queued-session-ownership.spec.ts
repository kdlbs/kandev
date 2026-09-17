import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  createQueuedSessionOwnershipScenario,
  expectSessionMessagesUnchanged,
  sessionMessageIds,
  waitForLaunchQueueCleared,
  waitForSessionState,
} from "./queued-session-ownership-helpers";

test.describe("mobile: queued session ownership", () => {
  test("keeps queue status visible while inspecting the parked conversation", async ({
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
        "Mobile queued ownership",
      );
      fillerSessionId = scenario.fillerSessionId;
      const sourceMessagesBefore = await sessionMessageIds(apiClient, scenario.sourceSessionId);

      await testPage.goto(`/t/${scenario.taskId}`);
      const sessionPage = new SessionPage(testPage);
      await sessionPage.waitForLoad();

      const mobileLayout = testPage.locator('[data-testid="mobile-task-layout"]:visible');
      const queueStatus = mobileLayout.locator('[data-testid="task-launch-queue-status"]:visible');
      await expect(queueStatus).toBeVisible();
      await expect(queueStatus).toContainText("Queued");
      await expect(queueStatus).toContainText("Waiting for session capacity");
      await expect(queueStatus).toContainText("1 of 1");
      await expect(queueStatus).toContainText(scenario.destinationProfileName);
      await assertNoDocumentHorizontalOverflow(testPage, "queued launch mobile detail");

      await mobileLayout.getByTestId("mobile-session-menu").tap();
      const taskDrawer = testPage.getByRole("dialog", { name: "Tasks" });
      const taskRow = taskDrawer.getByTestId("sidebar-task-item").filter({
        hasText: scenario.taskTitle,
      });
      await expect(taskRow).toBeVisible();
      await expect(taskRow.getByTestId("sidebar-task-launch-queue")).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "queued launch mobile task drawer");
      await testPage.keyboard.press("Escape");
      await expect(taskDrawer).not.toBeVisible();

      await mobileLayout.getByTestId("mobile-sessions-pill").tap();
      const sessionPicker = testPage.getByRole("dialog", { name: "Sessions" });
      const sourceRow = sessionPicker.getByTestId(`mobile-session-row-${scenario.sourceSessionId}`);
      await expect(sourceRow).toBeVisible();
      await expect(
        sessionPicker.getByTestId(`mobile-session-row-${scenario.destinationSessionId}`),
      ).toBeVisible();
      const sourceRowBox = await sourceRow.boundingBox();
      expect(sourceRowBox).not.toBeNull();
      expect(sourceRowBox!.height).toBeGreaterThanOrEqual(44);
      await sourceRow.tap();
      await expect(sessionPicker).not.toBeVisible();

      await expect(queueStatus).toBeVisible();
      await expect(
        testPage.locator('[data-testid="task-parked-session-note"]:visible'),
      ).toBeVisible();
      await expectSessionMessagesUnchanged(
        apiClient,
        scenario.sourceSessionId,
        sourceMessagesBefore,
      );

      await apiClient.stopSession({
        session_id: scenario.fillerSessionId,
        reason: "queued-session-ownership mobile e2e capacity release",
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
      await assertNoDocumentHorizontalOverflow(testPage, "queued launch mobile dispatched detail");
    } finally {
      if (fillerSessionId) {
        await apiClient
          .stopSession({
            session_id: fillerSessionId,
            reason: "queued-session-ownership mobile e2e cleanup",
            force: true,
          })
          .catch(() => undefined);
      }
      await releaseCeiling();
    }
  });
});
