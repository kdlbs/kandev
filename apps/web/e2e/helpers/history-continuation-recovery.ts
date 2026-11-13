import type { Page } from "@playwright/test";
import { expect, test } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import { assertDeliveryRecoveryGeometry } from "./delivery-recovery-controls";
import { waitForSessionDone } from "./session";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

type RecoveryRequest = { task_id: string; session_id: string; action: string };
type ContinuationResponseGate = { requestReceived: () => void; responseRelease: Promise<void> };

function createDeferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

async function interceptInterruptedRecovery(
  page: Page,
  sessionId: string,
  requests: RecoveryRequest[],
  continuationGate?: ContinuationResponseGate,
) {
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message !== "string") return server.send(message);
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part);
        if (
          frame.type !== "request" ||
          frame.action !== "session.recover" ||
          frame.payload?.session_id !== sessionId
        ) {
          server.send(part);
          continue;
        }
        requests.push(frame.payload);
        const continuing = frame.payload.action === "continue_from_history";
        const response = JSON.stringify({
          id: frame.id,
          type: continuing ? "response" : "error",
          action: frame.action,
          payload: continuing
            ? { success: true }
            : {
                code: "CONFLICT",
                message: "Session continuity requires explicit history continuation.",
                details: {
                  kind: "session_restore_required",
                  recovery_action: "continue_from_history",
                  reason: "unresolved_durable_work",
                  session_id: sessionId,
                },
              },
        });
        if (continuing && continuationGate) {
          continuationGate.requestReceived();
          void continuationGate.responseRelease.then(() => socket.send(response));
        } else {
          socket.send(response);
        }
      }
    });
    server.onMessage((message) => socket.send(message));
  });
}

// @covers AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2
export function historyContinuationRecoveryScenario() {
  test("blocked Resume exposes explicit history continuation in the composer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Interrupted clarification recovery",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const sessionId = task.session_id;
    if (!sessionId) throw new Error("fixture session missing");
    await waitForSessionDone(apiClient, task.id, sessionId, "history recovery fixture settled");
    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId,
      agentProfileId: seedData.agentProfileId,
      errorMessage: "Session could not resume",
      metadata: {
        last_agent_error: {
          message: "Session could not resume",
          stamp: "interrupted-clarification-fixture",
          scope: "session",
        },
      },
    });
    const requests: RecoveryRequest[] = [];
    await interceptInterruptedRecovery(testPage, sessionId, requests);
    if (testInfo.project.name === "mobile-chrome")
      await testPage.setViewportSize({ width: 320, height: 900 });
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();
    const card = testPage.getByTestId("session-recovery-card");
    await expect(card).toBeVisible();
    const continuation = card.getByTestId("recovery-continue-from-history-button");
    await expect(continuation).toHaveCount(0);
    expect(requests).toHaveLength(0);
    const resume = card.getByTestId("recovery-resume-button");
    if (testInfo.project.name === "mobile-chrome") await resume.tap();
    else await resume.click();
    await expect(continuation).toBeVisible();
    await expect(continuation).toBeEnabled();
    expect(
      await continuation.evaluate((button) => {
        const box = button.getBoundingClientRect();
        return button.contains(
          document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
        );
      }),
    ).toBe(true);
    expect(requests).toEqual([{ task_id: task.id, session_id: sessionId, action: "resume" }]);
    await assertNoDocumentHorizontalOverflow(testPage, "history continuation recovery");
    await testPage.screenshot({
      path: testInfo.outputPath("continue-from-history.png"),
      fullPage: true,
    });
    await prCapture.screenshot(`continue-from-history-${testInfo.project.name}`, {
      fullPage: true,
    });
    if (testInfo.project.name === "mobile-chrome") await continuation.tap();
    else await continuation.click();
    await expect
      .poll(() => requests)
      .toEqual([
        { task_id: task.id, session_id: sessionId, action: "resume" },
        { task_id: task.id, session_id: sessionId, action: "continue_from_history" },
      ]);
    await expect(continuation).toHaveCount(0);
  });

  test("uncertain delivery retains explicit history continuation", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Uncertain delivery continuation",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const sessionId = task.session_id;
    if (!sessionId) throw new Error("fixture session missing");
    await waitForSessionDone(apiClient, task.id, sessionId, "uncertain delivery fixture settled");
    await apiClient.seedTaskSession(task.id, {
      state: "FAILED",
      sessionId,
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
      completedAt: new Date().toISOString(),
      metadata: {
        last_agent_error: {
          message: "Delivery was interrupted.",
          code: "DURABLE_DELIVERY_UNCERTAIN",
          occurred_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
      },
    });

    const requests: RecoveryRequest[] = [];
    const continuationRequested = createDeferred();
    const continuationResponse = createDeferred();
    await interceptInterruptedRecovery(testPage, sessionId, requests, {
      requestReceived: continuationRequested.resolve,
      responseRelease: continuationResponse.promise,
    });
    const mobile = testInfo.project.name === "mobile-chrome";
    if (mobile) await testPage.setViewportSize({ width: 320, height: 900 });
    else await testPage.setViewportSize({ width: 360, height: 900 });
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();

    const banner = testPage.getByTestId("failed-session-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText(
      "The connection was interrupted. Kandev cannot confirm whether the agent processed your message.",
    );
    const retry = banner.getByTestId("recovery-retry-connection-button");
    await expect(retry).toBeVisible();
    await expect(banner.getByTestId("recovery-continue-from-history-button")).toHaveCount(0);
    expect(requests).toEqual([]);
    if (mobile) await retry.tap();
    else await retry.click();
    await expect
      .poll(() => requests)
      .toEqual([{ task_id: task.id, session_id: sessionId, action: "retry_connection" }]);

    const continuation = banner.getByTestId("recovery-continue-from-history-button");
    await expect(continuation).toBeVisible();
    await expect(banner.getByTestId("provider-restored-resume-disclosure")).toContainText(
      "Saved history starts a new native conversation. The interrupted prompt outcome remains unknown.",
    );
    expect(requests.filter((request) => request.action === "continue_from_history")).toHaveLength(
      0,
    );
    await assertDeliveryRecoveryGeometry(testPage, "recovery-continue-from-history-button");
    if (!mobile) {
      expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(false);
    }
    expect(
      await continuation.evaluate((button) => {
        const bounds = button.getBoundingClientRect();
        return button.contains(
          document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2),
        );
      }),
    ).toBe(true);
    if (mobile) {
      await testPage.screenshot({
        path: testInfo.outputPath("uncertain-delivery-continuation.png"),
        fullPage: true,
      });
      await prCapture.screenshot("uncertain-delivery-continuation-phone", { fullPage: true });
    }

    try {
      if (mobile) await continuation.tap();
      else await continuation.click();
      await continuationRequested.promise;
      await expect(continuation).toBeDisabled();
      await expect(
        banner.getByRole("status").filter({ hasText: "Continuing from saved history..." }),
      ).toBeVisible();
      expect(requests).toEqual([
        { task_id: task.id, session_id: sessionId, action: "retry_connection" },
        { task_id: task.id, session_id: sessionId, action: "continue_from_history" },
      ]);
    } finally {
      continuationResponse.resolve();
    }
    await expect(banner.getByTestId("recovery-continue-from-history-button")).toHaveCount(0);
    expect(requests.filter((request) => request.action === "continue_from_history")).toHaveLength(
      1,
    );
  });
}
