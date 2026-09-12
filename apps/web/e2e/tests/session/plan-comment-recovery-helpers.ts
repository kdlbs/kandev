import type { Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

type Frame = {
  id?: string;
  type?: string;
  action?: string;
  payload?: { task_id?: string; id?: string };
};
type RecoveryOptions = {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile: boolean;
};
const LEGACY_ID = "33333333-3333-4333-8333-333333333333";
const FEEDBACK = "Keep this recovered feedback";

function frame(value: string): Frame | null {
  try {
    return JSON.parse(value) as Frame;
  } catch {
    return null;
  }
}

function matchesRequest(
  parsed: Frame | null,
  taskId: string,
  actions: string[],
): parsed is Frame & { id: string } {
  return (
    parsed?.type === "request" &&
    typeof parsed.id === "string" &&
    parsed.payload?.task_id === taskId &&
    actions.includes(parsed.action ?? "")
  );
}

/** Reject only the selected task's correlated reads/uploads; keep chat and other frames live. */
async function interceptPlanRequests(
  page: Page,
  taskId: string,
  actions: string[],
  failedCommentId?: string,
) {
  let blocked = true;
  let rejected = 0;
  let succeeded = 0;
  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    const requestIds = new Set<string>();
    ws.onMessage((message) => {
      if (typeof message !== "string") {
        server.send(message);
        return;
      }
      const forwarded: string[] = [];
      for (const part of message.split("\n")) {
        const parsed = frame(part);
        const matches = matchesRequest(parsed, taskId, actions);
        if (matches && blocked && (!failedCommentId || parsed.payload?.id === failedCommentId)) {
          rejected++;
          ws.send(
            JSON.stringify({
              type: "error",
              id: parsed.id,
              action: parsed.action,
              payload: {
                code: "internal_error",
                message: "Injected transient plan request failure",
              },
            }),
          );
        } else {
          if (matches) requestIds.add(parsed.id!);
          if (part) forwarded.push(part);
        }
      }
      if (forwarded.length) server.send(forwarded.join("\n"));
    });
    server.onMessage((message) => {
      if (typeof message === "string")
        for (const part of message.split("\n")) {
          const parsed = frame(part);
          if (parsed?.type === "response" && parsed.id && requestIds.delete(parsed.id)) succeeded++;
        }
      ws.send(message);
    });
  });
  return {
    rejected: () => rejected,
    succeeded: () => succeeded,
    release: () => {
      blocked = false;
    },
  };
}

async function seedRecoveryTask({ apiClient, seedData }: RecoveryOptions, withPlan: boolean) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Plan comment recovery",
    seedData.agentProfileId,
    {
      description: withPlan
        ? planScript("## Recovery plan\n\nPreserve feedback")
        : "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Expected a task session");
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.some(
          (session) =>
            session.id === task.session_id &&
            ["COMPLETED", "WAITING_FOR_INPUT"].includes(session.state),
        );
      },
      { timeout: 60_000 },
    )
    .toBe(true);
  if (withPlan)
    await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  return { ...task, session_id: task.session_id };
}

async function showComposer(page: Page, taskId: string) {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  // Do not use the reload-capable readiness helpers while proving recovery.
  await expect(session.activeChat()).toBeVisible({ timeout: 30_000 });
  await expect(session.anyIdleInput()).toBeVisible({ timeout: 30_000 });
  const editor = session
    .activeChat()
    .locator('.tiptap.ProseMirror[contenteditable="true"]')
    .first();
  await expect(editor).toBeEditable();
  return { session, editor };
}

async function submit(session: SessionPage, mobile: boolean) {
  if (mobile) await session.tapSubmitWhenReady();
  else await session.clickSubmitWhenReady();
}

export async function assertPlainSendDuringRecovery(options: RecoveryOptions, withPlan: boolean) {
  const { testPage, apiClient, mobile } = options;
  const task = await seedRecoveryTask(options, withPlan);
  const control = await interceptPlanRequests(testPage, task.id, [
    "task.plan.get",
    "task.plan.comments.list",
  ]);
  const { session, editor } = await showComposer(testPage, task.id);
  await expect.poll(control.rejected, { timeout: 15_000 }).toBeGreaterThan(0);
  const notice = testPage.getByTestId("plan-comment-migration-notice");
  await expect(notice).toHaveCount(0);
  const message = "Plain message while comment reads are unavailable";
  if (mobile) await editor.tap();
  else await editor.click();
  await editor.fill(message);
  await testPage.screenshot({ path: test.info().outputPath("plan-comment-recovery-empty.png") });
  await submit(session, mobile);
  await expect
    .poll(
      async () =>
        (await apiClient.listSessionMessages(task.session_id)).messages.filter(
          (entry) => entry.author_type === "user" && entry.content.includes(message),
        ).length,
      { timeout: 30_000 },
    )
    .toBe(1);
  await expect(editor).toHaveText("");
  control.release();
  await testPage.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect.poll(control.succeeded).toBeGreaterThan(0);
  await expect(notice).toHaveCount(0);
  if (mobile) await assertNoDocumentHorizontalOverflow(testPage, "empty plan comment recovery");
}

async function seedMixedFeedback(page: Page, sessionId: string) {
  const legacy = {
    sessionId,
    source: "plan",
    selectedText: "Preserve feedback",
    from: 1,
    to: 10,
    createdAt: "2026-09-02T00:00:00Z",
    status: "pending",
  };
  const diff = {
    id: "legacy-diff",
    sessionId,
    source: "diff",
    text: "Keep this diff feedback",
    filePath: "src/app.ts",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "code",
    createdAt: legacy.createdAt,
    status: "pending",
  };
  const rows = [
    { ...legacy, id: "44444444-4444-4444-8444-444444444444", text: "Already recovered feedback" },
    { ...legacy, id: LEGACY_ID, text: FEEDBACK },
    diff,
  ];
  await page.addInitScript(
    ({ sessionId, rows }) => {
      sessionStorage.setItem(`kandev.comments.${sessionId}`, JSON.stringify(rows));
    },
    { sessionId, rows },
  );
  return diff;
}

export async function assertLegacyRecoveryPreservesDraft(options: RecoveryOptions) {
  const { testPage, apiClient, mobile } = options;
  const task = await seedRecoveryTask(options, true);
  const control = await interceptPlanRequests(
    testPage,
    task.id,
    ["task.plan.comments.create"],
    LEGACY_ID,
  );
  const diff = await seedMixedFeedback(testPage, task.session_id);
  const { session, editor } = await showComposer(testPage, task.id);
  await expect.poll(control.rejected, { timeout: 15_000 }).toBeGreaterThanOrEqual(3);
  await expect(session.activeChat().getByText("1 plan comment", { exact: true })).toBeVisible();
  const notice = session.activeChat().getByTestId("plan-comment-migration-notice");
  await expect(notice).toBeVisible();
  await testPage.screenshot({
    path: test.info().outputPath("plan-comment-recovery-attention.png"),
  });
  if (mobile) {
    const retrySize = await notice
      .getByRole("button", { name: "Retry", exact: true })
      .boundingBox();
    expect(retrySize?.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "actionable plan comment recovery");
    await editor.tap();
  } else await editor.click();
  const message = "Retain this draft until feedback recovers";
  await editor.fill(message);
  await submit(session, mobile);
  await expect(testPage.getByText("Message not sent", { exact: true })).toBeVisible();
  await expect(editor).toHaveText(message);
  await editor.focus();
  control.release();
  // A browser resume is enough; no Retry, reload, or submit is invoked by recovery.
  await testPage.evaluate(() => {
    window.dispatchEvent(new Event("focus"));
    window.dispatchEvent(new Event("pageshow"));
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await expect.poll(control.succeeded, { timeout: 15_000 }).toBe(2);
  await expect(session.activeChat().getByText("2 plan comments", { exact: true })).toBeVisible();
  await expect(notice).toHaveCount(0);
  await expect(editor).toHaveText(message);
  await expect(editor).toBeFocused();
  expect(
    (await apiClient.listSessionMessages(task.session_id)).messages.some((entry) =>
      entry.content.includes(message),
    ),
  ).toBe(false);
  expect(
    await testPage.evaluate(
      (sessionId) => JSON.parse(sessionStorage.getItem(`kandev.comments.${sessionId}`) ?? "[]"),
      task.session_id,
    ),
  ).toEqual([diff]);
  expect(control.succeeded()).toBe(2);
  await submit(session, mobile);
  await expect
    .poll(
      async () =>
        (await apiClient.listSessionMessages(task.session_id)).messages.filter(
          (entry) =>
            entry.author_type === "user" &&
            entry.content.includes(message) &&
            entry.content.includes(FEEDBACK),
        ).length,
      { timeout: 30_000 },
    )
    .toBe(1);
}
