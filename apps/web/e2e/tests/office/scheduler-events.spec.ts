import { test, expect } from "../../fixtures/office-fixture";

/**
 * Reactive scheduler — event → run wire.
 *
 * The office scheduler subscribes to a small set of domain events
 * and enqueues a `office_runs` row each time:
 *
 *   - `task.updated.assignee` (TaskUpdated subscriber)  → reason=task_assigned
 *   - `task.comment.created` (TaskComment subscriber)   → reason=task_comment
 *
 * These specs drive the real subscriber path — no harness seedRun —
 * so an endpoint rename or a missing subscriber wiring trips a
 * failure end-to-end. We poll the per-agent runs list rather than
 * waiting on WS push so the assertion is robust to event-bus
 * timing.
 */

type RunRow = { id: string; reason: string; task_id?: string; comment_id?: string };

async function listAgentRuns(
  apiClient: { rawRequest: (m: string, u: string) => Promise<Response> },
  agentId: string,
): Promise<RunRow[]> {
  const res = await apiClient.rawRequest("GET", `/api/v1/office/agents/${agentId}/runs`);
  if (!res.ok) return [];
  const body = (await res.json()) as { runs?: RunRow[] };
  return body.runs ?? [];
}

test.describe("Office reactive scheduler", () => {
  test("assigning a task to an agent enqueues a task_assigned run", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    test.setTimeout(30_000);

    // Create a task without an assignee, then attach the CEO.
    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Scheduler — task_assigned wire",
      { workflow_id: officeSeed.workflowId },
    );
    await officeApi.assignTask(task.id, officeSeed.agentId);

    // Poll the agent's runs list — the subscriber path is async via
    // the event bus. We expect at least one task_assigned row whose
    // payload references the task we just created.
    await expect
      .poll(
        async () => {
          const runs = await listAgentRuns(apiClient, officeSeed.agentId);
          return runs.filter((r) => r.reason === "task_assigned" && r.task_id === task.id);
        },
        { timeout: 20_000, message: "no task_assigned run surfaced for the new assignee" },
      )
      .not.toEqual([]);
  });

  test("posting a user comment on a CEO-assigned task enqueues a task_comment run", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    test.setTimeout(30_000);

    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Scheduler — task_comment wire",
      { workflow_id: officeSeed.workflowId },
    );
    await officeApi.assignTask(task.id, officeSeed.agentId);

    // Wait for the task_assigned run to land first so we can
    // disambiguate it from the comment-driven one we're about to
    // trigger.
    await expect
      .poll(
        async () => {
          const runs = await listAgentRuns(apiClient, officeSeed.agentId);
          return runs.filter((r) => r.reason === "task_assigned" && r.task_id === task.id).length;
        },
        { timeout: 20_000 },
      )
      .toBeGreaterThan(0);

    // Post a real user comment via the office endpoint.
    await officeApi.createTaskComment(task.id, "Heads up: please pick this up.");

    await expect
      .poll(
        async () => {
          const runs = await listAgentRuns(apiClient, officeSeed.agentId);
          return runs.filter((r) => r.reason === "task_comment" && r.task_id === task.id);
        },
        { timeout: 20_000, message: "no task_comment run surfaced for the comment" },
      )
      .not.toEqual([]);
  });
});
