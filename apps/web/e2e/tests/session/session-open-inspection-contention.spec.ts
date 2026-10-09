import { expect, test } from "../../fixtures/test-base";
import { routeSessionOpenInspectionContention } from "../../helpers/session-open-inspection-contention";
import { waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("session open inspection contention", () => {
  test("keeps the task recoverable and resumes the retained conversation after retry", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Resume after inspection contention ${Date.now()}`,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("created task has no session_id");

    const sessionId = task.session_id;
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${task.id}`);
    await session.waitForLoad();
    await expect(session.activeChat()).toContainText("simple mock response", { timeout: 45_000 });
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect
      .poll(() => apiClient.getTask(task.id).then((result) => result.state))
      .toBe("REVIEW");

    const identityBefore = await apiClient.getQueueSessionIdentity(task.id, sessionId);
    const draft = "keep this draft through inspection contention";
    await session.idleInput().fill(draft);
    const stopped = await apiClient.stopSession({
      session_id: sessionId,
      reason: "E2E inspection contention recovery",
      force: true,
    });
    expect(stopped.success).toBe(true);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the retained session to stop before recovery",
    });

    // Cancellation is persisted before detached runtime teardown finishes.
    // Open only when status inspection can actually restore the retained workspace.
    await expect
      .poll(
        () =>
          apiClient.wsRequest("task.session.status", { task_id: task.id, session_id: sessionId }),
        { timeout: 30_000 },
      )
      .toMatchObject({
        is_agent_running: false,
        needs_workspace_restore: true,
        auto_resume_allowed: true,
      });

    const proxy = await routeSessionOpenInspectionContention(testPage, {
      taskId: task.id,
      sessionId,
      pendingDelayMs: 1200,
    });
    await testPage.reload();
    await session.waitForLoad();
    await expect.poll(() => proxy.restoreRequestCount(), { timeout: 10_000 }).toBeGreaterThan(0);
    const restoreRequestsBeforeResume = proxy.restoreRequestCount();
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 15_000 });
    await session.recoveryResumeButton().click();
    await expect.poll(() => proxy.interceptedResumeCount()).toBe(1);
    await expect(session.activeChat().getByText("Resuming...", { exact: true })).toBeVisible();
    await expect(
      session
        .activeChat()
        .getByText("This workspace is still being checked. Try resuming the session again.", {
          exact: true,
        }),
    ).toBeVisible({ timeout: 15_000 });

    expect(proxy.resumeRequestCount()).toBe(1);
    expect(proxy.restoreRequestCount(), proxy.scopedActions().join(", ")).toBe(
      restoreRequestsBeforeResume,
    );
    expect((await apiClient.getTask(task.id)).state).toBe("REVIEW");
    expect((await apiClient.getTaskSession(sessionId)).session.state).toBe("CANCELLED");
    expect(await apiClient.getQueueSessionIdentity(task.id, sessionId)).toEqual(identityBefore);
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(testPage.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);
    await expect(testPage.getByTestId("recovery-fresh-button")).toHaveCount(0);
    await expect(testPage.getByTestId("recovery-new-branch-button")).toHaveCount(0);
    await expect(session.recoveryError()).toHaveCount(0);

    proxy.allowResumeRetries();
    await session.recoveryResumeButton().click();
    await expect.poll(() => proxy.resumeRequestCount(), { timeout: 15_000 }).toBe(2);
    await expect.poll(() => proxy.forwardedResumeResponses().length, { timeout: 30_000 }).toBe(1);
    const retryResponse = JSON.parse(proxy.forwardedResumeResponses()[0]) as {
      type?: string;
      payload?: { session_id?: string; state?: string };
    };
    expect(retryResponse).toMatchObject({
      type: "response",
      payload: { session_id: sessionId, state: "WAITING_FOR_INPUT" },
    });
    const editor = session.activeChat().getByTestId("chat-input-editor");
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 60_000 });
    await expect(editor).toContainText(draft);

    const marker = `inspection-retry-${Date.now()}`;
    await editor.fill(`/e2e:simple-message ${marker}`);
    await expect(session.submitButton()).toBeEnabled();
    await session.submitButton().click();
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "user" && message.content.includes(marker),
        );
      })
      .toBe(true);
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 45_000 });
    expect(await apiClient.getQueueSessionIdentity(task.id, sessionId)).toEqual(identityBefore);
    await expect
      .poll(() => apiClient.getTask(task.id).then((result) => result.state))
      .toBe("REVIEW");
  });
});
