import { expect, test } from "../../fixtures/office-fixture";

type RoutineRun = {
  id: string;
  causation_id?: string;
  linked_task_id?: string;
  status: string;
};

type AgentRun = { id: string; reason: string };

async function routineRuns(
  officeApi: { listRoutineRuns(id: string): Promise<Record<string, unknown>> },
  id: string,
) {
  const result = await officeApi.listRoutineRuns(id);
  return (Array.isArray(result.runs) ? result.runs : []) as RoutineRun[];
}

async function waitForAgentIdle(
  officeApi: {
    getAgent(id: string): Promise<Record<string, unknown>>;
    updateAgentStatus(id: string, status: string): Promise<Record<string, unknown>>;
  },
  agentId: string,
) {
  await expect
    .poll(
      async () => {
        const status = (await officeApi.getAgent(agentId)).status;
        if (status !== "stopped") return status;

        // A previous run can finish its cleanup after the fixture's
        // beforeEach status reset and briefly put the shared agent back into
        // stopped. Re-arm it when that transient state is observed, then let
        // the next poll confirm the durable idle state.
        try {
          await officeApi.updateAgentStatus(agentId, "idle");
        } catch {
          // The scheduler may still own the transition. Keep polling so the
          // next observation can repair it once the ownership is released.
        }
        return status;
      },
      {
        timeout: 120_000,
        intervals: [250, 500, 1_000, 2_000],
        message: "Waiting for the routine agent to become idle",
      },
    )
    .toBe("idle");
}

async function listAgentRuns(
  officeApi: { rawRequest: (method: string, path: string) => Promise<Response> },
  agentId: string,
): Promise<AgentRun[]> {
  const response = await officeApi.rawRequest("GET", `/agents/${agentId}/runs?limit=100`);
  if (!response.ok) {
    throw new Error(`Listing runs for agent ${agentId} failed with HTTP ${response.status}`);
  }
  const result = (await response.json()) as { runs?: AgentRun[] };
  return result.runs ?? [];
}

async function findAgentRunForRoutine(
  officeApi: { rawRequest: (method: string, path: string) => Promise<Response> },
  agentId: string,
  routineId: string,
  seenRunIds: Set<string>,
): Promise<string> {
  const runs = await listAgentRuns(officeApi, agentId);
  for (const run of runs) {
    if (seenRunIds.has(run.id) || !run.reason.startsWith("routine_")) continue;
    const response = await officeApi.rawRequest("GET", `/agents/${agentId}/runs/${run.id}`);
    if (!response.ok) continue;
    const detail = (await response.json()) as { context_snapshot?: string };
    try {
      const context = JSON.parse(detail.context_snapshot ?? "{}") as { routine_id?: string };
      if (context.routine_id === routineId) return run.id;
    } catch {
      // Other routine runs may have legacy or empty context snapshots.
    }
  }
  return "";
}

test.describe("Office taskless routine sessions", () => {
  test("fires a real taskless routine twice without creating task rows", async ({
    officeApi,
    apiClient,
    officeSeed,
  }) => {
    // A taskless launch has two asynchronous schedulers in front of the mock
    // agent (the wakeup dispatcher and the Office run scheduler). Under the
    // busiest CI shards a claimed run can wait through several scheduler
    // cycles before the runtime is admitted. Keep the test bounded, but allow
    // that startup window to complete without relying on Playwright retries.
    test.setTimeout(720_000);
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

    const existing = await listAgentRuns(officeApi, officeSeed.agentId);
    const seen = new Set(existing.map((run) => run.id));
    const sessions: string[] = [];
    for (let attempt = 1; attempt <= 2; attempt += 1) {
      await waitForAgentIdle(officeApi, officeSeed.agentId);
      const response = await officeApi.runRoutine(routineId);
      if (response.status !== 200) {
        throw new Error(
          `manual routine fire returned ${response.status}: ${await response.text()}`,
        );
      }
      const fired = (await response.json()) as { run: RoutineRun };
      expect(fired.run.id).toBeTruthy();
      const routineRunId = fired.run.id;
      const expectedCausationId = fired.run.causation_id;
      expect(expectedCausationId, "routine fire causation ID").toBeTruthy();
      await expect
        .poll(() => routineRuns(officeApi, routineId), { timeout: 20_000 })
        .toHaveLength(attempt);
      await expect
        .poll(async () =>
          (await routineRuns(officeApi, routineId)).some((run) => run.id === routineRunId),
        )
        .toBe(true);
      let runId = "";
      let observedRuns: unknown[] = [];
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
            observedRuns = (result.runs ?? []) as unknown[];
            const run = (observedRuns as { id: string; causation_id?: string }[]).find(
              (candidate) =>
                candidate.causation_id === expectedCausationId && !seen.has(candidate.id),
            );
            runId = run?.id ?? "";
            if (!runId) {
              runId = await findAgentRunForRoutine(officeApi, officeSeed.agentId, routineId, seen);
            }
            return runId;
          },
          {
            timeout: 90_000,
            intervals: [250, 500, 1_000],
            message: `Waiting for routine ${routineId} to create an agent run`,
          },
        )
        .not.toBe("")
        .catch((error) => {
          throw new Error(
            `No live office run found for routine ${routineId} (causation ID ${expectedCausationId}): ${JSON.stringify(observedRuns)}`,
            { cause: error },
          );
        });
      seen.add(runId);
      const detailPath = `/agents/${officeSeed.agentId}/runs/${runId}`;
      await expect
        .poll(
          async () => {
            const result = await officeApi.rawRequest("GET", detailPath);
            if (!result.ok) {
              throw new Error(`run detail returned ${result.status}: ${await result.text()}`);
            }
            const detail = await result.json();
            return detail.status;
          },
          { timeout: 300_000, intervals: [1_000, 2_000, 5_000] },
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
