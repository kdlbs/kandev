import { expect, test } from "../../fixtures/office-fixture";

type RoutineRun = {
  id: string;
  linked_task_id?: string;
  status: string;
};

type AgentRun = {
  id: string;
  agent_profile_id: string;
  reason: string;
};

async function routineRuns(
  officeApi: { listRoutineRuns(id: string): Promise<Record<string, unknown>> },
  id: string,
) {
  const result = await officeApi.listRoutineRuns(id);
  return (Array.isArray(result.runs) ? result.runs : []) as RoutineRun[];
}

test.describe("Office taskless routine sessions", () => {
  test("fires a real taskless routine twice without creating task rows", async ({
    officeApi,
    apiClient,
    officeSeed,
    backend,
  }) => {
    test.setTimeout(300_000);
    const before = await apiClient.listTasks(officeSeed.workspaceId);
    const routine = await officeApi.createRoutine(officeSeed.workspaceId, {
      name: `Taskless E2E ${Date.now()}`,
      description: "Taskless routine session smoke test",
      assignee_agent_profile_id: officeSeed.agentId,
      // This test asserts that each manual fire creates a distinct agent
      // session. Keep that contract independent of unrelated in-flight
      // coordinator work in the shared office workspace.
      concurrency_policy: "always_create",
    });
    const routineId = routine.id as string;

    const existing = await officeApi.listRuns(officeSeed.workspaceId);
    const seenAgentRuns = new Set(
      ((existing.runs ?? []) as AgentRun[])
        .filter((run) => run.agent_profile_id === officeSeed.agentId)
        .map((run) => run.id),
    );
    const sessions: string[] = [];
    for (let attempt = 1; attempt <= 2; attempt += 1) {
      await backend.ensureReady();
      const response = await officeApi.runRoutine(routineId);
      expect(response.status).toBe(200);
      await expect
        .poll(() => routineRuns(officeApi, routineId), { timeout: 20_000 })
        .toHaveLength(attempt);
      let runId = "";
      await expect
        .poll(
          async () => {
            const result = await officeApi.listRuns(officeSeed.workspaceId);
            const run = (result.runs as AgentRun[] | undefined)?.find(
              (candidate) =>
                candidate.agent_profile_id === officeSeed.agentId &&
                !seenAgentRuns.has(candidate.id) &&
                candidate.reason.startsWith("routine_"),
            );
            runId = run?.id ?? "";
            return runId;
          },
          { timeout: 120_000, message: "routine dispatch did not create an agent run" },
        )
        .not.toBe("");
      seenAgentRuns.add(runId);
      const detailPath = `/agents/${officeSeed.agentId}/runs/${runId}`;
      await expect
        .poll(
          async () => {
            const result = await officeApi.rawRequest("GET", detailPath);
            if (!result.ok) return "";
            const detail = await result.json();
            return detail.status;
          },
          { timeout: 120_000, message: "routine agent run detail did not become available" },
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
