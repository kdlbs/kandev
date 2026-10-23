import { test, expect } from "../../fixtures/office-fixture";
import type { ApiClient } from "../../helpers/api-client";
import type { OfficeApiClient } from "../../helpers/office-api-client";

/**
 * Reactive scheduler — task mutation → run wire.
 *
 * Office dashboard mutations enqueue a run row through reactivity:
 *
 *   - assignee mutation (dashboard reactivity) → reason=task_assigned
 *   - comment creation (dashboard reactivity)   → reason=task_comment
 *
 * These specs drive the real HTTP mutation paths without harness run seeding.
 * We poll the durable per-agent runs list rather than waiting for scheduler
 * execution or WS delivery.
 */

type RunRow = { id: string; reason: string; task_id?: string; comment_id?: string };

type RunPage = {
  runs?: RunRow[];
  next_cursor?: string;
  next_id?: string;
};

type SchedulerOffice = {
  workspaceId: string;
  workspaceName: string;
  agentId: string;
  workflowId: string;
};

async function withIsolatedSchedulerOffice(
  officeApi: OfficeApiClient,
  apiClient: Pick<ApiClient, "listWorkspaces">,
  agentProfileId: string,
  run: (office: SchedulerOffice) => Promise<void>,
): Promise<void> {
  const workspaceName = `Scheduler E2E ${Date.now()}`;
  const { workspaceId, agentId } = await officeApi.completeOnboarding({
    workspaceName,
    taskPrefix: "SCHED",
    agentName: "CEO",
    agentProfileId,
    executorPreference: "local_pc",
  });
  try {
    const { workspaces } = await apiClient.listWorkspaces();
    const workflowId = workspaces.find(
      (workspace) => workspace.id === workspaceId,
    )?.office_workflow_id;
    if (!workflowId) throw new Error("expected onboarding to create an Office workflow");

    await run({ workspaceId, workspaceName, agentId, workflowId });
  } finally {
    await officeApi.deleteWorkspace(workspaceId, workspaceName);
  }
}

async function listAgentRuns(
  apiClient: { rawRequest: (m: string, u: string) => Promise<Response> },
  agentId: string,
): Promise<RunRow[]> {
  const runs: RunRow[] = [];
  let cursor = "";
  let cursorId = "";

  for (;;) {
    const query = new URLSearchParams({ limit: "100" });
    if (cursor) {
      query.set("cursor", cursor);
      if (cursorId) query.set("cursor_id", cursorId);
    }

    const res = await apiClient.rawRequest(
      "GET",
      `/api/v1/office/agents/${agentId}/runs?${query.toString()}`,
    );
    if (!res.ok) {
      throw new Error(`listAgentRuns failed (${res.status}): ${await res.text()}`);
    }

    const body = (await res.json()) as RunPage;
    runs.push(...(body.runs ?? []));
    if (!body.next_cursor) return runs;

    // The API returns a stable (requested_at, id) cursor pair. Guard against
    // a malformed response looping forever while still allowing old servers
    // that omit the tie-breaker ID to make progress by timestamp.
    if (body.next_cursor === cursor && (body.next_id ?? "") === cursorId) {
      return runs;
    }
    cursor = body.next_cursor;
    cursorId = body.next_id ?? "";
  }
}

test.describe("Office reactive scheduler", () => {
  test("assigning a task to an agent enqueues a task_assigned run", async ({
    apiClient,
    officeApi,
    backend,
    seedData,
  }) => {
    test.setTimeout(150_000);

    await backend.ensureReady();
    await withIsolatedSchedulerOffice(
      officeApi,
      apiClient,
      seedData.agentProfileId,
      async ({ workspaceId, agentId, workflowId }) => {
        const task = await officeApi.createTask(workspaceId, "Scheduler task_assigned wire", {
          workflow_id: workflowId,
        });
        const taskId = task.id as string;
        if (!taskId) throw new Error("expected Office task creation to return an id");
        await officeApi.assignTask(taskId, agentId);

        await expect
          .poll(
            async () => {
              const runs = await listAgentRuns(apiClient, agentId);
              return runs.filter((r) => r.reason === "task_assigned" && r.task_id === taskId);
            },
            { timeout: 120_000, message: "no task_assigned run surfaced for the new assignee" },
          )
          .not.toEqual([]);
      },
    );
  });

  test("posting a user comment on a CEO-assigned task enqueues a task_comment run", async ({
    apiClient,
    officeApi,
    seedData,
  }) => {
    test.setTimeout(150_000);

    await withIsolatedSchedulerOffice(
      officeApi,
      apiClient,
      seedData.agentProfileId,
      async ({ workspaceId, agentId, workflowId }) => {
        const task = await officeApi.createTask(workspaceId, "Scheduler task_comment wire", {
          workflow_id: workflowId,
        });
        const taskId = task.id as string;
        if (!taskId) throw new Error("expected Office task creation to return an id");
        await officeApi.assignTask(taskId, agentId);

        await expect
          .poll(
            async () => {
              const runs = await listAgentRuns(apiClient, agentId);
              return runs.filter((r) => r.reason === "task_assigned" && r.task_id === taskId)
                .length;
            },
            { timeout: 120_000 },
          )
          .toBeGreaterThan(0);

        await officeApi.createTaskComment(taskId, "Heads up: please pick this up.");

        await expect
          .poll(
            async () => {
              const runs = await listAgentRuns(apiClient, agentId);
              return runs.filter((r) => r.reason === "task_comment" && r.task_id === taskId);
            },
            { timeout: 120_000, message: "no task_comment run surfaced for the comment" },
          )
          .not.toEqual([]);
      },
    );
  });
});
