import type { Response } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

/**
 * E2E coverage for the per-comment run status badge on the office task
 * chat page. A user comment that triggers a `task_comment` wakeup
 * carries a small pill that mirrors the run lifecycle:
 *
 *   queued    -> "Queued" pill
 *   claimed   -> "Working…" pill with spinner
 *   finished  -> no badge (the agent reply lands moments later)
 *   failed    -> red "Failed" pill + tooltip with error_message
 *   cancelled -> "Cancelled" pill
 *
 * Gating: only on user-authored comments, only while runStatus !== finished,
 * and only while no agent comment with a later created_at exists.
 *
 * Each test seeds its own task + comment + run so they remain independent
 * even though the worker-scoped officeSeed is shared.
 *
 * Every badge state in here is rendered from one response: the office comments
 * read, which carries `runStatus` per comment. A WS `office.run.processed`
 * event does not repaint the badge by itself, it triggers that read. So each
 * assertion below waits on the comments response that actually carries the
 * status it is about to assert, instead of giving the badge a timeout budget
 * and hoping the round trip fits inside it.
 */
const COMMENTS_PATH = /^\/api\/v1\/office\/tasks\/[^/]+\/comments$/;

type CommentsBody = { comments: Array<{ runStatus?: string }> };

/** Match the comments read whose body already carries `runStatus`. */
function commentsCarrying(status: string) {
  return async (response: Response) => {
    const body = (await response.json()) as CommentsBody;
    return body.comments.some((comment) => comment.runStatus === status);
  };
}
test.describe("User comment run status badge", () => {
  test("user comment shows Queued badge when its task_comment run is queued", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Run badge — queued", {
      workflow_id: officeSeed.workflowId,
    });

    const comment = await apiClient.seedComment({
      taskId: task.id,
      authorType: "user",
      authorId: "user",
      body: "what's the current date?",
    });

    await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_comment",
      status: "queued",
      taskId: task.id,
      commentId: comment.comment_id,
      idempotencyKey: `task_comment:${comment.comment_id}`,
    });

    const commentsLoaded = waitForHttp(testPage, "GET", COMMENTS_PATH);
    await testPage.goto(`/office/tasks/${task.id}`);
    await commentsLoaded;
    await expect(testPage.getByText("what's the current date?")).toBeVisible();

    const badge = testPage.getByTestId("user-comment-run-badge");
    await expect(badge).toBeVisible();
    // The reactive scheduler subscribes to office_run events and may
    // promote a freshly-seeded queued row to `claimed` before the page
    // commits its first render of the badge. Both states are valid
    // initial renders of an active run; the next test exercises the
    // transition explicitly. Accept either and pin that the textual
    // pill is the matching active label.
    const initialStatus = await badge.getAttribute("data-status");
    expect(initialStatus).toMatch(/^(queued|claimed)$/);
    await expect(badge).toContainText(initialStatus === "queued" ? "Queued" : "Working");
  });

  test("badge transitions queued → claimed over WS without page reload", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Run badge — live transition", {
      workflow_id: officeSeed.workflowId,
    });

    const comment = await apiClient.seedComment({
      taskId: task.id,
      authorType: "user",
      authorId: "user",
      body: "kick off a run please",
    });

    const seeded = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_comment",
      status: "queued",
      taskId: task.id,
      commentId: comment.comment_id,
      idempotencyKey: `task_comment:${comment.comment_id}`,
    });

    const commentsLoaded = waitForHttp(testPage, "GET", COMMENTS_PATH);
    await testPage.goto(`/office/tasks/${task.id}`);
    await commentsLoaded;
    await expect(testPage.getByText("kick off a run please")).toBeVisible();

    const badge = testPage.getByTestId("user-comment-run-badge");
    await expect(badge).toBeVisible();
    // The reactive scheduler can promote a freshly-seeded queued row to
    // `claimed` before the page renders the first badge frame — under
    // 50ms in mock mode. Accept either initial state; the contract
    // under test is that a WS-pushed status update mutates the badge
    // *without* a page reload, not the exact starting state.
    const initialStatus = await badge.getAttribute("data-status");
    expect(initialStatus).toMatch(/^(queued|claimed)$/);

    // Drive the run to a terminal `cancelled` state via the harness
    // PATCH route. Using a terminal state instead of `claimed` makes
    // the assertion observable regardless of whether the scheduler
    // already auto-claimed — and `cancelled` won't be undone by the
    // dispatcher. The patchRun handler publishes office.run.processed
    // which the gateway fans out — no page.reload() should be needed.
    // Armed before the PATCH. Matching on the body rather than the route makes
    // this immune to an unrelated refetch landing first: only the read that
    // actually carries `cancelled` satisfies it.
    const cancelledDelivered = waitForHttp(testPage, "GET", COMMENTS_PATH, {
      predicate: commentsCarrying("cancelled"),
    });
    await apiClient.updateRunStatus(seeded.run_id, { status: "cancelled" });
    await cancelledDelivered;

    await expect(badge).toHaveAttribute("data-status", "cancelled");
    await expect(badge).toContainText("Cancelled");
  });

  test("failed run shows Failed pill with error_message tooltip", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Run badge — failed", {
      workflow_id: officeSeed.workflowId,
    });

    const comment = await apiClient.seedComment({
      taskId: task.id,
      authorType: "user",
      authorId: "user",
      body: "this one will fail",
    });

    // Seed queued first then PATCH to failed with error_message —
    // the seedRun POST route does not currently persist
    // error_message on insert (CreateRun's INSERT statement omits
    // the column), but the PATCH route writes it via
    // SetRunErrorMessageForTest. Driving the transition through
    // PATCH also exercises the office.run.processed event path.
    const seeded = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_comment",
      status: "queued",
      taskId: task.id,
      commentId: comment.comment_id,
      idempotencyKey: `task_comment:${comment.comment_id}`,
    });
    await apiClient.updateRunStatus(seeded.run_id, {
      status: "failed",
      errorMessage: "agent failed: budget exceeded",
    });

    const commentsLoaded = waitForHttp(testPage, "GET", COMMENTS_PATH, {
      predicate: commentsCarrying("failed"),
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await commentsLoaded;
    await expect(testPage.getByText("this one will fail")).toBeVisible();

    const badge = testPage.getByTestId("user-comment-run-badge");
    await expect(badge).toBeVisible();
    await expect(badge).toHaveAttribute("data-status", "failed");
    await expect(badge).toContainText("Failed");

    // Surface the Radix tooltip. Hover the trigger and assert the
    // portal-rendered content carries the error message.
    await badge.scrollIntoViewIfNeeded();
    await badge.hover();
    const tooltip = testPage.locator('[data-slot="tooltip-content"]');
    await expect(tooltip).toBeVisible({ timeout: 10_000 });
    await expect(tooltip).toContainText("agent failed: budget exceeded");
  });

  test("badge is hidden once a later agent reply exists", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Run badge — replied", {
      workflow_id: officeSeed.workflowId,
    });

    // Seed user comment with an explicit older timestamp so the agent
    // reply we seed next reliably has created_at > comment.createdAt.
    const earlier = new Date(Date.now() - 60_000).toISOString();
    const userComment = await apiClient.seedComment({
      taskId: task.id,
      authorType: "user",
      authorId: "user",
      body: "did you get this?",
      createdAt: earlier,
    });

    // Run is still queued, but a later agent comment already exists
    // — the gating logic must hide the badge regardless.
    await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_comment",
      status: "queued",
      taskId: task.id,
      commentId: userComment.comment_id,
      idempotencyKey: `task_comment:${userComment.comment_id}`,
    });

    await apiClient.seedComment({
      taskId: task.id,
      authorType: "agent",
      authorId: officeSeed.agentId,
      body: "yes, on it",
      source: "session",
    });

    const commentsLoaded = waitForHttp(testPage, "GET", COMMENTS_PATH);
    await testPage.goto(`/office/tasks/${task.id}`);
    await commentsLoaded;
    await expect(testPage.getByText("did you get this?")).toBeVisible();
    await expect(testPage.getByText("yes, on it")).toBeVisible();

    // The user comment carries a queued runStatus, but the agent
    // reply with a later created_at should suppress the badge.
    await expect(testPage.getByTestId("user-comment-run-badge")).toHaveCount(0);
  });
});
