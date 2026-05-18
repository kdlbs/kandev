import { test, expect } from "../../fixtures/office-fixture";

/**
 * Comment-driven resume + UI deeplink chain.
 *
 * Office's reactive scheduler enqueues a `task_comment` run whenever
 * a new user comment lands on a task with an assigned agent. The
 * runs list surface in the Agent detail page then deeplinks each
 * row to its triggering entity:
 *
 *   reason=task_comment + task_id + comment_id
 *     → `/office/tasks/<task>#comment-<comment>`
 *
 * This spec exercises the chain end-to-end without needing a live
 * scheduler: we seed a run with the same payload shape the
 * dispatcher would produce, then drive the UI to confirm:
 *
 *   1. The Linked column in the Runs list renders the deeplink.
 *   2. Clicking it navigates to `/office/tasks/<task>#comment-<comment>`.
 *   3. The comment anchor (id=comment-<id>) is in the DOM so the
 *      browser's hash-scroll behaviour can land on it.
 */
test.describe("Task comment resume run", () => {
  test("runs list deeplinks task_comment runs to the originating comment", async ({
    apiClient,
    testPage,
    officeSeed,
  }) => {
    test.setTimeout(60_000);

    // Create a task in the office workspace and seed a user comment
    // on it. The harness comment route accepts source="user" /
    // author_type="user" so we get a real row with an id we can
    // reference from the run payload.
    const task = await apiClient.createTask(officeSeed.workspaceId, "Comment-resume task", {
      workflow_id: officeSeed.workflowId,
    });
    const comment = await apiClient.seedComment({
      taskId: task.id,
      authorType: "user",
      authorId: "user",
      body: "Please pick this up.",
    });

    // Seed a `task_comment` run pointing at the (task, comment) pair.
    // The runtime would set agent_profile_id to whoever is assigned;
    // we use officeSeed.agentId so the run shows up under the CEO's
    // /office/agents/<id>/runs page.
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_comment",
      status: "finished",
      taskId: task.id,
      commentId: comment.comment_id,
    });

    // Navigate to the runs page and confirm the row is present with
    // a Linked-column deeplink whose href targets the comment.
    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs`);
    const row = testPage.getByTestId(`agent-run-row-${run.run_id}`);
    await expect(row).toBeVisible({ timeout: 10_000 });

    const linkedDeeplink = row.locator(
      `a[href*="/office/tasks/${task.id}#comment-${comment.comment_id}"]`,
    );
    await expect(linkedDeeplink).toBeVisible({ timeout: 5_000 });

    // Click the deeplink and confirm the task page lands with the
    // expected hash and the matching anchor in the DOM.
    await linkedDeeplink.click();
    await testPage.waitForURL(`**/office/tasks/${task.id}#comment-${comment.comment_id}`);

    const anchor = testPage.locator(`#comment-${comment.comment_id}`);
    await expect(anchor).toBeAttached({ timeout: 10_000 });
    // The comment body is rendered through the markdown component so
    // we assert the text content sticks around as another sanity.
    await expect(anchor).toContainText("Please pick this up.");
  });
});
