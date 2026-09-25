import { expect, test } from "../../fixtures/office-fixture";

type RoutineRun = {
  id: string;
  linked_task_id?: string;
  status: string;
};

type AgentRun = {
  id: string;
  reason: string;
  context_snapshot?: string;
};

async function routineRuns(
  officeApi: { listRoutineRuns(id: string): Promise<Record<string, unknown>> },
  id: string,
) {
  const result = await officeApi.listRoutineRuns(id);
  return (Array.isArray(result.runs) ? result.runs : []) as RoutineRun[];
}

async function waitForAgentIdle(
  officeApi: { getAgent(id: string): Promise<Record<string, unknown>> },
  agentId: string,
) {
  await expect
    .poll(async () => (await officeApi.getAgent(agentId)).status, {
      timeout: 30_000,
      message: "Waiting for the routine agent to become idle",
    })
    .toBe("idle");
}

test.describe("Office taskless routine sessions", () => {
  test("fires a real taskless routine twice without creating task rows", async ({
    officeApi,
    apiClient,
    officeSeed,
  }) => {
    test.setTimeout(420_000);
    // The worker resets the status before each test, but the status write and
    // scheduler claim are asynchronous. Do not fire a routine while the
    // previous run still holds the agent in a transient working state.
    await waitForAgentIdle(officeApi, officeSeed.agentId);
    const before = await apiClient.listTasks(officeSeed.workspaceId);
    const routine = await officeApi.createRoutine(officeSeed.workspaceId, {
      name: `Taskless E2E ${Date.now()}`,
      description: "Taskless routine session smoke test",
      assignee_agent_profile_id: officeSeed.agentId,
      concurrency_policy: "always_create",
    });
    const routineId = routine.id as string;

    const existing = await officeApi.listRuns(officeSeed.workspaceId);
    const seen = new Set(((existing.runs ?? []) as { id: string }[]).map((run) => run.id));
    const sessions: string[] = [];
    for (let attempt = 1; attempt <= 2; attempt += 1) {
      await waitForAgentIdle(officeApi, officeSeed.agentId);
      const response = await officeApi.runRoutine(routineId);
      expect(response.status).toBe(200);
      let runId = "";
      await expect
        .poll(() => routineRuns(officeApi, routineId), {
          timeout: 30_000,
          intervals: [250, 500, 1_000],
          message: `Waiting for routine run ${attempt} to appear`,
        })
        .toHaveLength(attempt);
      await expect
        .poll(
          async () => {
            const result = await officeApi.listRuns(officeSeed.workspaceId);
            const run = ((result.runs ?? []) as AgentRun[]).find((candidate) => {
              if (seen.has(candidate.id) || !candidate.reason.startsWith("routine_")) {
                return false;
              }
              try {
                return JSON.parse(candidate.context_snapshot ?? "{}").routine_id === routineId;
              } catch {
                return false;
              }
            });
            runId = run?.id ?? "";
            return runId;
          },
          {
            timeout: 60_000,
            intervals: [250, 500, 1_000],
            message: `Waiting for agent run ${attempt} to appear`,
          },
        )
        .not.toBe("");
      seen.add(runId);
      const detailPath = `/agents/${officeSeed.agentId}/runs/${runId}`;
      await expect
        .poll(
          async () => {
            const result = await officeApi.rawRequest("GET", detailPath);
            expect(result.ok).toBe(true);
            const detail = await result.json();
            return detail.status;
          },
          { timeout: 180_000 },
        )
        .toMatch(/^(finished|failed|cancelled)$/);
      const detail = await (await officeApi.rawRequest("GET", detailPath)).json();
      expect({ status: detail.status, error: detail.error_message ?? "" }).toEqual({
        status: "finished",
        error: "",
      });
      expect(detail.task_id ?? "").toBe("");
      expect(detail.session.session_id).toBeTruthy();
      expect(detail.assembled_prompt).toBeTruthy();
      sessions.push(detail.session.session_id);
    }

    const runs = await routineRuns(officeApi, routineId);
    expect(runs.every((run) => !run.linked_task_id)).toBe(true);
    expect(new Set(runs.map((run) => run.id)).size).toBe(2);

    expect(new Set(sessions).size).toBe(2);
    const after = await apiClient.listTasks(officeSeed.workspaceId);
    expect(after.tasks.map((task) => task.id)).toEqual(before.tasks.map((task) => task.id));
  });
});
