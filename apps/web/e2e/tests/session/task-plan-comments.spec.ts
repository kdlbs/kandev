import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { waitForStableActiveSession } from "../../helpers/session-store";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const PLAN_CONTENT = "## Shared plan\n\nReview this shared implementation step";

async function waitForSessions(apiClient: ApiClient, taskId: string, count: number) {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions.filter((session) => DONE_STATES.includes(session.state)).length;
      },
      { timeout: 60_000 },
    )
    .toBe(count);
}

async function seedTwoSessionTask(page: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Task-owned plan comments",
    seedData.agentProfileId,
    {
      description: planScript(PLAN_CONTENT),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  await session.waitForChatIdle({ timeout: 45_000 });
  await session.openNewSessionDialog();
  await session.newSessionPromptInput().fill("/e2e:simple-message");
  await session.newSessionStartButton().click();
  await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });
  await waitForSessions(apiClient, task.id, 2);
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const primary = sessions.find((candidate) => candidate.is_primary);
  const secondary = sessions.find((candidate) => !candidate.is_primary);
  if (!primary || !secondary) throw new Error("expected primary and secondary sessions");
  return { task, session, primary, secondary };
}

async function openPlanCommentComposer(page: Page, session: SessionPage) {
  const editor = session.planPanel.locator(".ProseMirror:visible");
  await expect(editor).toBeVisible({ timeout: 10_000 });
  await editor.click();
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+a`);
  await page.keyboard.press(`${modifier}+Shift+c`);
  const textarea = page.locator(
    'textarea[placeholder="Add your comment or instruction..."]:visible',
  );
  await expect(textarea).toBeVisible({ timeout: 5_000 });
  return textarea;
}

test.describe("task-owned plan comments", () => {
  // @covers AC-TASKS-PLAN-COMMENTS-001.2
  // @covers AC-TASKS-PLAN-COMMENTS-002.2
  // @covers AC-TASKS-PLAN-COMMENTS-002.5
  // @covers AC-TASKS-PLAN-COMMENTS-003.1
  // @covers AC-TASKS-PLAN-COMMENTS-003.2
  test("shares comments across tabs while Send and Run keep distinct destinations", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { task, session, primary, secondary } = await seedTwoSessionTask(
      testPage,
      apiClient,
      seedData,
    );

    await session.togglePlanMode();
    await expect(
      testPage.locator('[data-testid="plan-comment-migration-notice"]:visible'),
    ).toHaveCount(0, { timeout: 15_000 });
    const textarea = await openPlanCommentComposer(testPage, session);
    const sendComment = "Send this feedback only to the selected secondary session";
    await textarea.fill(sendComment);
    await testPage.getByRole("button", { name: "Add", exact: true }).click();
    await expect(session.planPanel.locator(".comment-badge[data-comment-id]")).toHaveCount(1);

    await session.sessionTabBySessionId(primary.id).click();
    await waitForStableActiveSession(testPage, primary.id);
    await expect(session.activeChat().getByText("1 plan comment", { exact: true })).toBeVisible();
    await session.sessionTabBySessionId(secondary.id).click();
    await waitForStableActiveSession(testPage, secondary.id);
    await expect(session.activeChat().getByText("1 plan comment", { exact: true })).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await waitForStableActiveSession(testPage, secondary.id);
    await expect(session.activeChat().getByText("1 plan comment", { exact: true })).toBeVisible();
    await session.sendMessage("Apply the selected feedback here");

    await expect
      .poll(
        async () =>
          (await apiClient.listSessionMessages(secondary.id)).messages.some(
            (message) =>
              message.author_type === "user" &&
              message.content.includes(sendComment) &&
              message.content.includes("Apply the selected feedback here"),
          ),
        { timeout: 30_000 },
      )
      .toBe(true);
    expect(
      (await apiClient.listSessionMessages(primary.id)).messages.some((message) =>
        message.content.includes(sendComment),
      ),
    ).toBe(false);
    await expect(session.planPanel.locator(".comment-badge[data-comment-id]")).toHaveCount(0, {
      timeout: 15_000,
    });
    await session.waitForChatIdle({ timeout: 45_000 });
    // A completed seed turn can auto-promote the secondary. Pin the intended
    // primary so this flow isolates selected-session Send from primary Run.
    await apiClient.setPrimarySession(primary.id);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).primary_session_id, {
        timeout: 15_000,
      })
      .toBe(primary.id);
    await expect(session.primaryStarInSessionTab(primary.id)).toBeVisible({ timeout: 15_000 });
    await session.sessionTabBySessionId(secondary.id).click();
    await waitForStableActiveSession(testPage, secondary.id);

    const runTextarea = await openPlanCommentComposer(testPage, session);
    const runComment = "Run this feedback in the primary session";
    await runTextarea.fill(runComment);
    const run = testPage.getByRole("button", { name: "Run", exact: true });
    await expect(run).toBeEnabled();
    await run.click();

    await expect
      .poll(
        async () =>
          (await apiClient.listSessionMessages(primary.id)).messages.some(
            (message) => message.author_type === "user" && message.content.includes(runComment),
          ),
        { timeout: 30_000 },
      )
      .toBe(true);
    expect(
      (await apiClient.listSessionMessages(secondary.id)).messages.some((message) =>
        message.content.includes(runComment),
      ),
    ).toBe(false);
    await expect(session.planPanel.locator(".comment-badge[data-comment-id]")).toHaveCount(0, {
      timeout: 15_000,
    });
  });
});
