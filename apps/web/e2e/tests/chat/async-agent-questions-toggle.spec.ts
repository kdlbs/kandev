import { test, expect } from "../../fixtures/test-base";
import { openTaskSession, waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { seedClarificationSession } from "../../helpers/clarification";
import {
  ASYNC_QUESTIONS_ENV,
  clearAsyncQuestionsOverride,
  expectBootAsyncQuestions,
  openAsyncQuestionsToggle,
  readAsyncQuestionsFlag,
  saveAsyncQuestionsOverride,
} from "./async-agent-questions-toggle-helpers";

test.describe("Async agent questions release toggle", () => {
  test("defaults off while blocking clarification remains usable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await clearAsyncQuestionsOverride(testPage, backend);
    await testPage.reload();
    const flag = await readAsyncQuestionsFlag(testPage, backend);
    expect(flag.effective_value).toBe(false);
    expect(flag.override_value).toBeNull();
    expect(flag.env_locked).toBe(false);

    const { toggle } = await openAsyncQuestionsToggle(testPage);
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(toggle).toBeEnabled();
    await prCapture.screenshot("desktop-async-questions-toggle-off", {
      caption: "Async agent questions are off by default in Feature Toggles",
    });

    const envLock = await backend.useEnv({ [ASYNC_QUESTIONS_ENV]: "false" });
    try {
      await testPage.reload();
      const locked = await readAsyncQuestionsFlag(testPage, backend);
      expect(locked.env_locked).toBe(true);
      expect(locked.source).toBe("env");
      const lockedToggle = testPage
        .getByTestId("feature-toggle-features.asyncQuestions")
        .getByRole("switch");
      await expect(lockedToggle).toBeDisabled();
      await expect(lockedToggle).toHaveAttribute("aria-checked", "false");
      const patch = await testPage.request.patch(
        `${backend.baseUrl}/api/v1/runtime-flags/features.asyncQuestions`,
        { data: { override: true } },
      );
      expect(patch.status()).toBe(409);
    } finally {
      await envLock();
    }

    await testPage.reload();
    const requestUrls: string[] = [];
    testPage.on("request", (request) => {
      if (new URL(request.url()).pathname.startsWith("/api/v1/agent-questions")) {
        requestUrls.push(request.url());
      }
    });
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Blocking Questions With Async Off",
      { scenario: "clarification" },
    );
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 20_000 });
    await expect(session.clarificationOverlay()).toContainText("Which database");
    await expect(testPage.getByTestId("task-async-question-strip")).toHaveCount(0);
    expect(requestUrls).toEqual([]);
    await session.clarificationOption("PostgreSQL").click();
    await session.submitClarification();
    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });

    await clearAsyncQuestionsOverride(testPage, backend);
  });

  test("retains an eligible request across off and on restarts", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(240_000);
    await clearAsyncQuestionsOverride(testPage, backend);
    await testPage.reload();
    try {
      const { toggle: initialToggle } = await openAsyncQuestionsToggle(testPage);
      await expect(initialToggle).toHaveAttribute("aria-checked", "false");
      await expect(initialToggle).toBeEnabled();
      await prCapture.screenshot("desktop-async-questions-toggle-off-before-opt-in", {
        caption: "The installation can opt in from the existing Feature Toggles card",
      });

      await saveAsyncQuestionsOverride(testPage, true);
      await expect(testPage.getByText("Restart required")).toBeVisible();
      expect((await readAsyncQuestionsFlag(testPage, backend)).override_value).toBe(true);
      await prCapture.screenshot("desktop-async-questions-toggle-saved", {
        caption: "The saved opt-in shows the existing restart-required state",
      });

      await backend.restart();
      await testPage.reload();
      await expectBootAsyncQuestions(testPage, true);
      const enabledFlag = await readAsyncQuestionsFlag(testPage, backend);
      expect(enabledFlag.effective_value).toBe(true);
      expect(enabledFlag.source).toBe("override");
      await prCapture.screenshot("desktop-async-questions-toggle-on-after-restart", {
        caption: "The browser receives async questions in the restarted boot payload",
      });

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Async Question Retained During Rollback",
        seedData.agentProfileId,
        {
          description: "/e2e:async-question-wait",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("expected a session for the async rollback flow");
      await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WAIT_WORK_MARKER", 60_000);
      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId: task.session_id,
        expectedState: "WAITING_FOR_INPUT",
        message: "the async question should be open before disabling its feature",
        timeout: 60_000,
      });
      await expect
        .poll(() =>
          apiClient.getTask(task.id).then((current) => current.primary_session_pending_action),
        )
        .toBe("clarification");
      await testPage.goto("/needs-you-inbox");
      const group = testPage
        .getByTestId("async-question-group")
        .filter({ hasText: "Async Question Retained During Rollback" });
      await expect(group).toBeVisible({ timeout: 30_000 });
      await expect(group.getByTestId("async-question-row").first()).toBeVisible();
      await group.getByTestId("async-question-dismiss").click();
      await expect(group).toHaveCount(0, { timeout: 20_000 });
      const hiddenPanel = testPage.getByTestId("needs-you-inbox-hidden-panel");
      await expect(hiddenPanel).toBeVisible({ timeout: 20_000 });
      await hiddenPanel.getByRole("button", { name: /Show hidden/ }).click();
      const hiddenRow = hiddenPanel.getByTestId("needs-you-inbox-hidden-async-row");
      await expect(hiddenRow).toBeVisible({ timeout: 20_000 });
      await prCapture.screenshot("desktop-async-question-before-disable", {
        caption: "The pending async request is retained in the shared Inbox hidden panel",
      });

      await saveAsyncQuestionsOverride(testPage, false);
      await expect(testPage.getByText("Restart required")).toBeVisible();
      await backend.restart();
      await testPage.reload();
      await expectBootAsyncQuestions(testPage, false);
      const disabledFlag = await readAsyncQuestionsFlag(testPage, backend);
      expect(disabledFlag.effective_value).toBe(false);
      expect(disabledFlag.override_value).toBe(false);
      await expect
        .poll(() =>
          apiClient.getTask(task.id).then((current) => current.primary_session_pending_action),
        )
        .toBeNull();

      const asyncRoutes: string[] = [];
      testPage.on("request", (request) => {
        if (new URL(request.url()).pathname.startsWith("/api/v1/agent-questions")) {
          asyncRoutes.push(request.url());
        }
      });
      await testPage.goto("/needs-you-inbox");
      await expect(testPage.getByTestId("needs-you-inbox-empty")).toBeVisible({ timeout: 20_000 });
      const disabledHiddenPanel = testPage.getByTestId("needs-you-inbox-hidden-panel");
      if ((await disabledHiddenPanel.count()) > 0) {
        await disabledHiddenPanel.getByRole("button", { name: /Show hidden/ }).click();
        await expect(disabledHiddenPanel.getByText("Nothing is currently hidden.")).toBeVisible({
          timeout: 20_000,
        });
        await expect(
          disabledHiddenPanel.getByTestId("needs-you-inbox-hidden-async-row"),
        ).toHaveCount(0);
      }
      await expect(
        testPage
          .getByTestId("async-question-group")
          .filter({ hasText: "Async Question Retained During Rollback" }),
      ).toHaveCount(0);
      const session = await openTaskSession(testPage, task.id);
      await session.waitForDirectInput(30_000);
      await expect(testPage.getByTestId("agent-question-message")).toHaveCount(0);
      await expect(testPage.getByTestId("task-async-question-strip")).toHaveCount(0);
      expect(asyncRoutes).toEqual([]);
      await prCapture.screenshot("desktop-async-question-hidden-after-disable", {
        caption: "Retained async content is hidden after the off restart",
      });
      await session.sendMessageViaButton(
        'e2e:message("Normal prompt accepted while async questions are off")',
      );
      await waitForAgentMessage(
        apiClient,
        task.session_id,
        "Normal prompt accepted while async questions are off",
        60_000,
      );

      await saveAsyncQuestionsOverride(testPage, true);
      await backend.restart();
      await testPage.reload();
      await expectBootAsyncQuestions(testPage, true);
      await testPage.goto("/needs-you-inbox");
      const restoredHiddenPanel = testPage.getByTestId("needs-you-inbox-hidden-panel");
      await expect(restoredHiddenPanel).toBeVisible({ timeout: 20_000 });
      await restoredHiddenPanel.getByRole("button", { name: /Show hidden/ }).click();
      const restoredHiddenRow = restoredHiddenPanel.getByTestId("needs-you-inbox-hidden-async-row");
      await expect(restoredHiddenRow).toBeVisible({ timeout: 20_000 });
      await restoredHiddenRow.getByRole("button", { name: /Restore/ }).click();
      const restoredGroup = testPage
        .getByTestId("async-question-group")
        .filter({ hasText: "Async Question Retained During Rollback" });
      await expect(restoredGroup).toBeVisible({ timeout: 30_000 });
      const restoredRow = restoredGroup.getByTestId("async-question-row").first();
      await restoredRow.getByTestId("async-question-row-toggle").click();
      await restoredRow
        .getByTestId("async-question-option")
        .filter({ hasText: "PostgreSQL" })
        .click();
      await restoredRow.getByTestId("async-question-submit").click();
      await expect(restoredGroup).toHaveCount(0, { timeout: 30_000 });
      await prCapture.screenshot("desktop-async-question-answer-after-reenable", {
        caption: "The retained request can be answered after re-enabling and restarting",
      });
    } finally {
      await clearAsyncQuestionsOverride(testPage, backend);
    }
  });
});
