import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { routeNavigationResponses } from "../../helpers/navigation-response-hold";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  assertRestartRetained,
  captureRestartBaseline,
  createRestartScenario,
} from "./session-tab-restart-helpers";

// @covers AC-TASKS-SESSION-TAB-RESTART-001.4
test("blocks delete-mode tab close throughout restart", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const baseline = (await apiClient.getUserSettings()).settings.agent_tab_close_behavior;
  const gate = await routeNavigationResponses(testPage);
  try {
    await apiClient.saveUserSettings({ agent_tab_close_behavior: "delete_session" });
    const fixture = await createRestartScenario(testPage, apiClient, seedData);
    await apiClient.launchSession({
      task_id: fixture.task.id,
      agent_profile_id: seedData.agentProfileId,
      prompt: "/e2e:simple-message",
    });
    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(fixture.task.id)).sessions.filter(
            (session) => session.state === "WAITING_FOR_INPUT",
          ).length,
      )
      .toBe(2);
    const tab = fixture.session.sessionTabBySessionId(fixture.sessionId);
    await tab.click();
    const close = testPage.getByTestId(`session-tab-close-${fixture.sessionId}`);
    await expect(close).toBeVisible();
    await expect(close).toHaveAccessibleName("Delete session");
    const before = await captureRestartBaseline(apiClient, fixture.task.id, fixture.sessionId);
    gate.hold((request) => request.action === "session.stop");
    await tab.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).click();
    await expect.poll(() => gate.heldCount()).toBe(1);
    await expect
      .poll(async () => (await apiClient.getTaskSession(fixture.sessionId)).session.state)
      .toBe("CANCELLED");
    await expect(testPage.getByTestId("session-restart-status")).toBeVisible();
    await expect(close).toBeHidden();
    if (prCapture.capturing) {
      await waitForFiniteAnimations(testPage.locator("body"));
      await prCapture.screenshot("desktop-delete-close-blocked", {
        caption: "Delete-mode tab close stays unavailable throughout session restart.",
      });
    }

    const launched = fixture.wire.waitForResponse("session.launch");
    gate.release();
    gate.hold((request) => request.action === "session.launch");
    await expect.poll(() => gate.heldCount()).toBe(1);
    await expect(close).toBeHidden();
    gate.release();
    await launched;
    await assertRestartRetained(apiClient, fixture.task.id, fixture.sessionId, before);
    await expect(testPage.getByTestId("session-restart-status")).not.toBeVisible();
    await expect(close).toBeVisible();
  } finally {
    gate.release();
    await apiClient.saveUserSettings({ agent_tab_close_behavior: baseline });
  }
});

test.describe("Session tab close guard", () => {
  test("last session tab hides the close button", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);

    // Create a task with one session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Single Session Close Guard",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    // Find the session tab
    const sessionTab = testPage.locator("[data-testid^='session-tab-']").first();
    await expect(sessionTab).toBeVisible({ timeout: 10_000 });

    // The close action inside the session tab should be hidden
    // (DockviewDefaultTab renders .dv-default-tab-action but hideClose hides it via CSS)
    const closeAction = sessionTab.locator(".dv-default-tab-action");
    await expect(closeAction).toBeHidden({ timeout: 5_000 });
  });
});
