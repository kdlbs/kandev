// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { seedIdleSession } from "../../helpers/session";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile: pause queue recovery", () => {
  test.describe.configure({ retries: 1 });

  test("force-stop parks the next message until Run next is tapped", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // This spec covers pause/resume queue semantics, not workflow completion.
    // Opt out explicitly because the Kanban template enables cancellation
    // completion by default.
    await apiClient.updateWorkflowStep(seedData.startStepId, {
      cancel_triggers_turn_complete: false,
    });

    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile pause parks queue",
    );
    const chat = session.activeChat();

    await session.sendMessageViaButton("/slow 8s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    const editor = chat.locator(".tiptap.ProseMirror");
    await typeWhileBusy(testPage, editor, "/e2e:simple-message");
    await chat.getByTestId("submit-message-button").tap();

    const queueChip = chat.getByTestId("queue-chip");
    await expect(queueChip).toBeVisible({ timeout: 10_000 });

    await session.cancelAgentButton().tap();

    await expect(session.agentStatus()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(queueChip).toBeVisible();
    await expect(chat.getByText("simple mock response", { exact: false }).nth(1)).not.toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    await queueChip.tap();
    const runNext = chat.getByTestId("queue-drain-next");
    await expect(runNext).toBeVisible();
    await expect(runNext).toBeInViewport();
    await runNext.tap();

    await expect(chat.getByTestId("queue-chip")).not.toBeVisible({ timeout: 30_000 });
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
  });

  test("a compact stalled notice remains touch-safe without expanding to full width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile stalled notice recovery",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "RUNNING",
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "status",
      content: "Still waiting on Start dev server.",
      metadata: {
        action_visibility: "running",
        actions: [
          {
            type: "ws_request",
            label: "Cancel turn",
            test_id: "stall-cancel-turn-button",
            params: { method: "agent.cancel", payload: { session_id: sessionId } },
          },
        ],
      },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const notice = session.activeChat().getByTestId("running-action-notice");
    const cancel = notice.getByTestId("stall-cancel-turn-button");

    await expect(notice).toBeVisible();
    const [noticeBox, cancelBox] = await Promise.all([notice.boundingBox(), cancel.boundingBox()]);
    expect(noticeBox).not.toBeNull();
    expect(cancelBox).not.toBeNull();
    expect(noticeBox!.height).toBeLessThanOrEqual(56);
    expect(cancelBox!.height).toBeGreaterThanOrEqual(44);
    expect(cancelBox!.width).toBeLessThan(noticeBox!.width);
    await assertNoDocumentHorizontalOverflow(testPage);

    await cancel.tap();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(notice).not.toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
