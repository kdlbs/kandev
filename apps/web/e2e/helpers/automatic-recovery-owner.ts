import { test, expect } from "../fixtures/test-base";
import { waitForSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";
import type { Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export function automaticRecoveryOwnerScenario() {
  test("automatic recovery has one owner without bootstrap metadata", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Automatic recovery owner",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("fixture session missing");
    const sessionId = task.session_id;
    await waitForSessionDone(apiClient, task.id, sessionId, "automatic recovery fixture settled");
    await apiClient.seedTaskSession(task.id, {
      sessionId,
      state: "FAILED",
      completedAt: new Date().toISOString(),
      agentProfileId: seedData.agentProfileId,
      errorMessage: "Connection interrupted",
      metadata: {},
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "status",
      content: "Earlier recovery history stays available",
    });
    const attempts = await failAutomaticRecovery(testPage, task.id, sessionId);
    await testPage.goto(`/t/${task.id}`);
    await expect.poll(() => ({ ...attempts })).toMatchObject({ status: 1, resume: 1, restore: 1 });
    const card = testPage.getByTestId("session-recovery-card");
    await expect(card).toHaveCount(1);
    await expect(card).toBeVisible();
    await expect(
      testPage
        .locator('[data-testid="session-recovery-error"]')
        .filter({ has: testPage.locator('[role="alert"]') }),
    ).toHaveCount(0);
    await expect(testPage.getByTestId("ensure-session-error-banner")).toHaveCount(0);
    await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(1);
    await card.getByText("Technical details", { exact: true }).click();
    await expect(card.locator("pre")).toContainText("Automatic resume transport failed");
    await expect(card.locator("pre")).toContainText("Automatic workspace restore failed");
    await expect(
      testPage.getByText("Earlier recovery history stays available", { exact: true }),
    ).toBeVisible();
    await revealRecoveryFromFiles(testPage, testInfo.project.name === "mobile-chrome");
    await assertNoDocumentHorizontalOverflow(testPage, "automatic recovery owner");
    if (testInfo.project.name === "mobile-chrome") {
      const box = await card.getByTestId("recovery-resume-button").boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
    await testPage.screenshot({
      path: testInfo.outputPath("automatic-recovery-owner.png"),
      fullPage: true,
    });
    const resume = card.getByTestId("recovery-resume-button");
    if (testInfo.project.name === "mobile-chrome") await resume.tap();
    else await resume.click();
    await expect.poll(() => attempts.manual).toBe(1);
    await expect(resume).toBeDisabled();
    await expect(card.getByTestId("recovery-fresh-button")).toBeDisabled();
    attempts.finishManual();
    await expect(card.getByTestId("session-recovery-error")).toBeVisible();
    await expect(resume).toBeEnabled();
    await expect(card).toHaveCount(1);
  });
}

async function revealRecoveryFromFiles(page: Page, mobile: boolean) {
  if (mobile)
    await page
      .getByTestId("session-mobile-bottom-nav")
      .getByRole("button", { name: "Files", exact: true })
      .tap();
  else await new SessionPage(page).clickTab("Files");
  const link = page
    .getByTestId("files-panel")
    .getByRole("link", { name: "View recovery", exact: true });
  await expect(link).toHaveCount(1);
  if (mobile) await link.tap();
  else await link.click();
  await expect(page.getByTestId("session-recovery-card")).toBeFocused();
}

async function failAutomaticRecovery(page: Page, taskId: string, sessionId: string) {
  const attempts = { resume: 0, restore: 0, status: 0, manual: 0, finishManual: () => {} };
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message !== "string") return server.send(message);
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part);
        if (frame.type !== "request" || frame.payload?.session_id !== sessionId) {
          server.send(part);
          continue;
        }
        let payload;
        if (frame.action === "session.recover") {
          attempts.manual++;
          attempts.finishManual = () =>
            socket.send(
              JSON.stringify({
                id: frame.id,
                type: "response",
                action: frame.action,
                payload: { success: false, error: "Manual resume transport failed" },
              }),
            );
          continue;
        } else if (frame.action === "task.session.status") {
          attempts.status++;
          payload = {
            task_id: taskId,
            session_id: sessionId,
            state: "FAILED",
            is_agent_running: false,
            is_resumable: true,
            needs_resume: true,
            auto_resume_allowed: true,
          };
        } else if (frame.action === "session.launch") {
          const restore = frame.payload.intent === "restore_workspace";
          if (restore) attempts.restore++;
          else attempts.resume++;
          payload = {
            success: false,
            task_id: taskId,
            session_id: sessionId,
            state: "FAILED",
            error: restore
              ? "Automatic workspace restore failed"
              : "Automatic resume transport failed",
          };
        } else {
          server.send(part);
          continue;
        }
        socket.send(
          JSON.stringify({ id: frame.id, type: "response", action: frame.action, payload }),
        );
      }
    });
    server.onMessage((message) => socket.send(message));
  });
  return attempts;
}
