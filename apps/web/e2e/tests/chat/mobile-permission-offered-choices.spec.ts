import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Provider-offered permission choices on mobile", () => {
  test("shows every Codex decision with a reachable touch target", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Codex approval choices", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "permission_request",
      content: "Codex wants to run git status",
      metadata: {
        request_id: "req-mobile-codex-choice",
        pending_id: "pend-mobile-codex-choice",
        tool_call_id: "item-mobile-codex-choice",
        action_type: "command",
        action_details: { command: "git status" },
        status: "pending",
        options: [
          {
            option_id: "codex-allow-once",
            name: "Approve once",
            kind: "allow_once",
            metadata: { codex_app_server: true, codex_decision: "accept" },
          },
          {
            option_id: "codex-command-policy",
            name: "Approve with command policy",
            kind: "allow_always",
            metadata: {
              codex_app_server: true,
              codex_decision: "accept_with_execpolicy_amendment",
            },
          },
          {
            option_id: "codex-deny",
            name: "Deny",
            kind: "reject_once",
            metadata: { codex_app_server: true, codex_decision: "decline" },
          },
          {
            option_id: "codex-cancel",
            name: "Cancel turn",
            kind: "reject_once",
            metadata: { codex_app_server: true, codex_decision: "cancel" },
          },
        ],
      },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const actionRow = session.activeChat().getByTestId("permission-action-row");
    await expect(actionRow).toBeVisible();
    const choices = ["Approve", "Approve with command rule", "Deny", "Cancel"];
    for (let index = 0; index < choices.length; index++) {
      const button = actionRow.getByTestId(`permission-choice-${index}`);
      await expect(button).toHaveText(choices[index]);
      const box = await button.boundingBox();
      expect(box, `${choices[index]} should have a visible touch target`).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      expect(box!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    }

    await actionRow.getByTestId("permission-choice-2").tap();
    await expect(actionRow).toHaveCount(0);
  });
});
