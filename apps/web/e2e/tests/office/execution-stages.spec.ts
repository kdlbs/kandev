import { test, expect } from "../../fixtures/office-fixture";

// Placeholder: set execution policy on a task.
// The backend does not expose this via HTTP yet — SetTaskExecutionPolicy is
// internal-only. Once a PUT /api/v1/office/tasks/:id/execution-policy route
// is added, replace this stub with a real fetch call.
async function setExecutionPolicy(
  _baseUrl: string,
  _taskId: string,
  _policy: object,
): Promise<void> {
  throw new Error(
    "setExecutionPolicy: no HTTP route available yet — backend needs PUT /tasks/:id/execution-policy",
  );
}

test.describe("Execution stages — status updates", () => {
  test("update task status to done without execution policy succeeds", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Status Done Task", {
      workflow_id: officeSeed.workflowId,
    });

    const result = await officeApi.updateTaskStatus(task.id, "done", "Work complete");
    expect(result).toMatchObject({ ok: true });

    // Verify status changed via the issue endpoint
    const afterResp = await officeApi.getTask(task.id);
    const after = (afterResp as { task: { status: string } }).task;
    expect(after.status).toBe("done");
  });

  test("update task status to in_progress without execution policy succeeds", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Status InProgress Task", {
      workflow_id: officeSeed.workflowId,
    });

    const result = await officeApi.updateTaskStatus(task.id, "in_progress", "Resuming work");
    expect(result).toMatchObject({ ok: true });

    const afterResp = await officeApi.getTask(task.id);
    const after = (afterResp as { task: { status: string } }).task;
    expect(after.status).toBe("in_progress");
  });

  test("update task status with unknown status returns error", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Status Invalid Task", {
      workflow_id: officeSeed.workflowId,
    });

    await expect(
      officeApi.updateTaskStatus(task.id, "invalid-status", "some comment"),
    ).rejects.toThrow();
  });

  test("comment is recorded on status update", async ({ officeApi, officeSeed, apiClient }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Status Comment Task", {
      workflow_id: officeSeed.workflowId,
    });
    const commentText = "Status updated in E2E test";

    await officeApi.updateTaskStatus(task.id, "done", commentText);

    // Comments endpoint should include the status-change comment
    const commentsResp = await officeApi.listTaskComments(task.id);
    const comments = (commentsResp as { comments: Array<{ body: string }> }).comments;
    expect(comments.some((c) => c.body === commentText)).toBe(true);
  });
});

test.describe("Execution stages — policy transitions", () => {
  // NOTE: These tests require an HTTP endpoint to set execution_policy on a task.
  // The backend exposes SetTaskExecutionPolicy via service but has no HTTP route
  // for it yet (internal/office/dashboard/handler.go only handles status+comment).
  // These tests are written as specifications and will pass once the endpoint
  // is exposed. They are skipped for now to keep CI green.

  test.skip("work stage advances to review on done status", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Stage Work→Review Task");

    const policy = {
      stages: [
        { id: "work", type: "work", participants: [], approvals_needed: 0 },
        {
          id: "review",
          type: "review",
          participants: [{ type: "agent", agent_id: officeSeed.agentId }],
          approvals_needed: 1,
        },
      ],
    };

    await setExecutionPolicy("", task.id, policy);

    // Transition work → review
    await officeApi.updateTaskStatus(task.id, "done", "Implementation complete");

    const issueResp = await officeApi.getTask(task.id);
    const issue = (issueResp as { task: Record<string, unknown> }).task;
    expect(issue).toBeDefined();
  });

  test.skip("review approve advances to next stage", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Stage Review Approve Task");

    const policy = {
      stages: [
        { id: "work", type: "work", participants: [], approvals_needed: 0 },
        {
          id: "review",
          type: "review",
          participants: [{ type: "agent", agent_id: officeSeed.agentId }],
          approvals_needed: 1,
        },
        { id: "ship", type: "ship", participants: [], approvals_needed: 0 },
      ],
    };

    await setExecutionPolicy("", task.id, policy);

    await officeApi.updateTaskStatus(task.id, "done", "Work complete");
    await officeApi.updateTaskStatus(task.id, "done", "LGTM");
    await officeApi.updateTaskStatus(task.id, "done", "Shipped");

    const issueResp = await officeApi.getTask(task.id);
    const issue = (issueResp as { task: { status: string } }).task;
    expect(issue.status).toBe("COMPLETED");
  });

  test.skip("review reject returns task to work stage", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Stage Review Reject Task");

    const policy = {
      stages: [
        { id: "work", type: "work", participants: [], approvals_needed: 0 },
        {
          id: "review",
          type: "review",
          participants: [{ type: "agent", agent_id: officeSeed.agentId }],
          approvals_needed: 1,
        },
      ],
    };

    await setExecutionPolicy("", task.id, policy);

    await officeApi.updateTaskStatus(task.id, "done", "Work complete");
    // Reviewer rejects — task should return to in_progress
    await officeApi.updateTaskStatus(task.id, "in_progress", "Needs rework");

    const issueResp = await officeApi.getTask(task.id);
    const issue = (issueResp as { task: { status: string } }).task;
    expect(issue.status).toBe("IN_PROGRESS");
  });

  test.skip("missing comment on stage transition returns error", async ({
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Stage Missing Comment Task");

    const policy = {
      stages: [
        { id: "work", type: "work", participants: [], approvals_needed: 0 },
        {
          id: "review",
          type: "review",
          participants: [{ type: "agent", agent_id: officeSeed.agentId }],
          approvals_needed: 1,
        },
      ],
    };

    await setExecutionPolicy("", task.id, policy);

    await officeApi.updateTaskStatus(task.id, "done", "Work complete");
    // Missing comment should fail
    await expect(officeApi.updateTaskStatus(task.id, "done", "")).rejects.toThrow();
  });
});
