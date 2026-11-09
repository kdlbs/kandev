// AC-UI-NEEDS-YOU-INBOX-001.30: listing an answerable bundle, expanding it,
// and answering it in place must remove the row (the convergence re-read)
// and resume the blocked task, without ever navigating to the task page.
import { createHash } from "node:crypto";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";

test.describe("Needs-you Inbox answer flow", () => {
  test("answering a row removes it and resumes the task without navigation (AC .30)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const prCapture = new PrAssetCapture(
      testPage,
      path.join(__dirname, "needs-you-inbox-answer.spec.ts"),
    );
    const title = "Needs-you Inbox Answer Flow";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the Inbox answer flow");

    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should reach its blocking state before the Inbox is opened",
      timeout: 60_000,
    });

    // Never visits /t/:id: the row is listed, expanded, and answered entirely
    // from the Inbox route.
    await testPage.addInitScript(() => {
      const originalFetch = window.fetch;
      window.fetch = (input, init) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/api/v1/attachments") && init?.body instanceof FormData) {
          window.sessionStorage.setItem(
            "e2e.attachment-upload-workspace-id",
            String(init.body.get("workspace_id") ?? ""),
          );
        }
        return originalFetch(input, init);
      };
    });
    await testPage.goto("/needs-you-inbox");

    const row = testPage.getByTestId("needs-you-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 30_000 });
    await prCapture.screenshot("inbox-list", { caption: "Needs-you Inbox list" });

    await row.getByTestId("needs-you-inbox-row-toggle").click();
    const overlay = row.getByTestId("clarification-overlay");
    await expect(overlay).toBeVisible({ timeout: 15_000 });
    await prCapture.screenshot("inbox-row-expanded", {
      caption: "Row expanded to answer in place",
    });

    await overlay.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" }).click();
    await overlay.getByTestId("clarification-custom-submit").click();

    // The outcome callback fires a convergence re-read; the row disappears
    // because the answered bundle is no longer answerable, not because the
    // client spliced it out locally.
    await expect(row).toHaveCount(0, { timeout: 15_000 });

    // `WAITING_FOR_INPUT` is the session's normal resting state once a turn
    // ends, so it is reached both while blocked on the clarification AND
    // again once the agent's post-answer turn finishes -- it cannot
    // distinguish "still blocked" from "resumed and settled". The mock
    // agent's own post-answer message is the unambiguous proof the blocked
    // tool call was actually released and the turn ran to completion.
    await waitForAgentMessage(apiClient, task.session_id, "You answered:", 30_000);
    prCapture.flush();
  });
});

test.describe("Needs-you Inbox answer attachments", () => {
  test("uploads a file from the inbox row and preserves its source task scope", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Inbox Answer Attachment Scope";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification-files",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected a clarification session");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should reach its blocking state before the Inbox is opened",
      timeout: 60_000,
    });
    await testPage.addInitScript(() => {
      const originalFetch = window.fetch;
      window.fetch = (input, init) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/api/v1/attachments") && init?.body instanceof FormData) {
          window.sessionStorage.setItem(
            "e2e.attachment-upload-workspace-id",
            String(init.body.get("workspace_id") ?? ""),
          );
        }
        return originalFetch(input, init);
      };
    });
    await testPage.goto("/needs-you-inbox");
    const row = testPage.getByTestId("needs-you-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 30_000 });
    await row.getByTestId("needs-you-inbox-row-toggle").click();
    const overlay = row.getByTestId("clarification-overlay");
    await expect(overlay).toBeVisible({ timeout: 15_000 });

    await overlay.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" }).click();
    const upload = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    );
    const png = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/6X3pZQAAAAASUVORK5CYII=",
      "base64",
    );
    await overlay.getByTestId("clarification-attachment-input").setInputFiles({
      name: "inbox.png",
      mimeType: "image/png",
      buffer: png,
    });
    expect((await upload).ok()).toBe(true);
    await expect(overlay.getByTestId("clarification-attachment-badge")).toHaveText("1");
    await expect
      .poll(() =>
        testPage.evaluate(() =>
          window.sessionStorage.getItem("e2e.attachment-upload-workspace-id"),
        ),
      )
      .toBe(seedData.workspaceId);
    await overlay.getByTestId("clarification-custom-submit").click();

    await expect(row).toHaveCount(0, { timeout: 15_000 });
    const digest = createHash("sha256").update(png).digest("hex");
    await waitForAgentMessage(
      apiClient,
      task.session_id,
      `Attachment proof: inbox.png ${png.length} ${digest}`,
      30_000,
    );
  });
});
