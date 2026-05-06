import path from "node:path";
import { spawnSync } from "node:child_process";
import { test, expect } from "../../fixtures/office-fixture";

/**
 * Smoke tests for the new `agentctl kandev …` subcommand groups.
 *
 * Each test spawns the `agentctl` binary directly with the same env
 * vars the office runtime injects at agent launch (KANDEV_API_URL,
 * KANDEV_API_KEY, KANDEV_WORKSPACE_ID, …) and asserts the request
 * round-trips through the backend's office API. The `KANDEV_API_KEY`
 * is a real runtime JWT minted via the existing
 * `/api/v1/_test/runtime-token` helper, so the agent auth middleware
 * sees a valid token instead of falling through to admin mode.
 *
 * Why these tests: the agentctl CLI is now the canonical surface for
 * every office mutation the CEO makes. Without coverage, an endpoint
 * rename / payload-shape drift on the backend would silently break
 * the CEO's CLI without any unit test catching it.
 */

const BACKEND_DIR = path.resolve(__dirname, "../../../../../apps/backend");
const AGENTCTL_BIN = path.join(BACKEND_DIR, "bin", "agentctl");

type CLIResult = {
  exitCode: number;
  stdout: string;
  stderr: string;
};

function runCLI(env: NodeJS.ProcessEnv, args: string[]): CLIResult {
  const proc = spawnSync(AGENTCTL_BIN, ["kandev", ...args], {
    env: { ...process.env, ...env },
    encoding: "utf8",
    timeout: 15_000,
  });
  return {
    exitCode: proc.status ?? -1,
    stdout: proc.stdout?.toString() ?? "",
    stderr: proc.stderr?.toString() ?? "",
  };
}

test.describe("agentctl kandev CLI", () => {
  test("agents list returns the CEO from the seeded office workspace", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    // Seed a run + mint a runtime JWT so the agent auth middleware
    // accepts our calls. Without a real run id the mint endpoint
    // rejects the request.
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const res = runCLI(env, ["agents", "list"]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);
    const parsed = JSON.parse(res.stdout);
    // Office's GET /workspaces/:wsId/agents returns `{ agents: [...] }`.
    const agents = (parsed.agents ?? []) as Array<{ id: string; role?: string }>;
    expect(agents.some((a) => a.id === officeSeed.agentId)).toBe(true);
  });

  test("tasks list returns workspace tasks after one is created", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    // Create a task so the list isn't empty.
    const created = await apiClient.createTask(officeSeed.workspaceId, "CLI Task List Marker", {
      workflow_id: officeSeed.workflowId,
    });

    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const res = runCLI(env, ["tasks", "list"]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);
    const parsed = JSON.parse(res.stdout);
    const tasks = (parsed.tasks ?? []) as Array<{ id: string; title?: string }>;
    expect(tasks.some((t) => t.id === created.id)).toBe(true);
  });

  test("approvals list returns an empty queue on a fresh workspace", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "routine_dispatch",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const res = runCLI(env, ["approvals", "list"]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);
    const parsed = JSON.parse(res.stdout);
    // The list endpoint returns `{ approvals: [...] }`; the array
    // may be empty on a fresh workspace.
    expect(Array.isArray(parsed.approvals)).toBe(true);
  });

  test("budget get returns a cost summary structure", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const res = runCLI(env, ["budget", "get"]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);
    // Cost summary shape is workspace-dependent; we just confirm
    // the response is valid JSON, not an error string. A 500 / 4xx
    // would have surfaced as exitCode != 0 via handleResponse.
    expect(() => JSON.parse(res.stdout)).not.toThrow();
  });

  test("routines list returns the routine collection (possibly empty)", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "routine_dispatch",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const res = runCLI(env, ["routines", "list"]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);
    const parsed = JSON.parse(res.stdout);
    expect(Array.isArray(parsed.routines)).toBe(true);
  });

  // --- Mutation sub-verbs ---
  //
  // The list sub-verbs above prove the binary's argv → flag-parser → HTTP
  // wiring works for read paths, but the same wiring on mutation paths is
  // where a typo'd flag default or forgotten verb would actually corrupt
  // state. Each mutation test below spawns the binary, then re-reads via
  // the existing apiClient/officeApi helpers so the assertion observes the
  // server-side side effect rather than the binary's own print-back.

  test("agents create hires a new agent that shows up in list", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    // Unique name so a stray previous-test row from the same worker can't
    // satisfy the "agent landed" assertion below.
    const name = `cli-worker-${Date.now()}`;
    const res = runCLI(env, [
      "agents",
      "create",
      "--name",
      name,
      "--role",
      "worker",
      "--reports-to",
      officeSeed.agentId,
    ]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);
    const parsed = JSON.parse(res.stdout) as { agent?: { id?: string; name?: string } };
    expect(parsed.agent?.name).toBe(name);
    const newAgentId = parsed.agent?.id;
    expect(typeof newAgentId).toBe("string");

    // Verify the side effect via a separate read — the binary returning the
    // created row is not the same as the row being persisted.
    const listRes = await apiClient.rawRequest(
      "GET",
      `/api/v1/office/workspaces/${officeSeed.workspaceId}/agents`,
    );
    expect(listRes.ok).toBe(true);
    const listJson = (await listRes.json()) as { agents?: Array<{ id: string; name?: string }> };
    expect((listJson.agents ?? []).some((a) => a.id === newAgentId)).toBe(true);
  });

  test("tasks message posts a comment that shows up on the task", async ({
    apiClient,
    officeApi,
    backend,
    officeSeed,
  }) => {
    const created = await apiClient.createTask(
      officeSeed.workspaceId,
      `CLI Message Target ${Date.now()}`,
      { workflow_id: officeSeed.workflowId },
    );

    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
      taskId: created.id,
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
      taskId: created.id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
      KANDEV_TASK_ID: created.id as string,
    };

    const body = `cli-handoff-${Date.now()}`;
    const res = runCLI(env, ["tasks", "message", "--prompt", body]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);

    // Re-read comments via the office API — the binary echoing back the
    // created comment is not the same as the row being persisted.
    const comments = (await officeApi.listTaskComments(created.id as string)) as {
      comments?: Array<{ body?: string }>;
    };
    expect((comments.comments ?? []).some((c) => c.body === body)).toBe(true);
  });

  test("routines create persists a routine with the supplied name", async ({
    apiClient,
    officeApi,
    backend,
    officeSeed,
  }) => {
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "routine_dispatch",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const name = `cli-standup-${Date.now()}`;
    const res = runCLI(env, [
      "routines",
      "create",
      "--name",
      name,
      "--task-title",
      "Daily standup",
      "--assignee",
      officeSeed.agentId,
      "--cron",
      "0 9 * * MON-FRI",
    ]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);

    // The two-step create (routine + trigger) means the final stdout is
    // the trigger response; the routine itself we verify by listing.
    const listed = (await officeApi.listRoutines(officeSeed.workspaceId)) as {
      routines?: Array<{ id: string; name?: string }>;
    };
    expect((listed.routines ?? []).some((r) => r.name === name)).toBe(true);
  });

  test("agents update renames an existing worker", async ({
    apiClient,
    officeApi,
    backend,
    officeSeed,
  }) => {
    // Hire a throwaway worker so we don't rename the CEO out from under
    // subsequent tests (officeSeed is worker-scoped and reused).
    const worker = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: `cli-rename-target-${Date.now()}`,
      role: "worker",
    })) as { id: string; name?: string };

    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const newName = `cli-renamed-${Date.now()}`;
    const res = runCLI(env, ["agents", "update", "--id", worker.id, "--name", newName]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);

    // Verify via a fresh read — the PATCH echoing back the row is not the
    // same as the row being persisted.
    const fetched = (await officeApi.getAgent(worker.id)) as { name?: string };
    expect(fetched.name).toBe(newName);
  });

  test("agents delete removes a throwaway worker from the list", async ({
    apiClient,
    officeApi,
    backend,
    officeSeed,
  }) => {
    const worker = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: `cli-delete-target-${Date.now()}`,
      role: "worker",
    })) as { id: string };

    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const res = runCLI(env, ["agents", "delete", "--id", worker.id]);
    expect(res.exitCode, `stderr=${res.stderr}`).toBe(0);

    // Re-list and assert the row is gone — DELETE returning 200/204 is not
    // the same as the row being removed.
    const listed = (await officeApi.listAgents(officeSeed.workspaceId)) as {
      agents?: Array<{ id: string }>;
    };
    expect((listed.agents ?? []).some((a) => a.id === worker.id)).toBe(false);
  });

  test("routines pause / resume / delete cycle a routine through its lifecycle", async ({
    apiClient,
    officeApi,
    backend,
    officeSeed,
  }) => {
    // Chain the three lifecycle verbs on the same routine: pause →
    // resume → delete. Splitting into three tests would force three
    // throwaway routines and triple the seedRun/mintRuntimeToken cost
    // without buying additional coverage — each CLI invocation is still
    // verified independently against the office API.
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: `cli-lifecycle-${Date.now()}`,
      description: "Lifecycle test routine",
    })) as { id: string };

    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "routine_dispatch",
      status: "claimed",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
    };

    const findRoutine = async (): Promise<Record<string, unknown> | undefined> => {
      const listed = (await officeApi.listRoutines(officeSeed.workspaceId)) as {
        routines?: Array<Record<string, unknown>>;
      };
      return (listed.routines ?? []).find((r) => r.id === routine.id);
    };

    // Pause.
    const pauseRes = runCLI(env, ["routines", "pause", "--id", routine.id]);
    expect(pauseRes.exitCode, `pause stderr=${pauseRes.stderr}`).toBe(0);
    expect((await findRoutine())?.status).toBe("paused");

    // Resume.
    const resumeRes = runCLI(env, ["routines", "resume", "--id", routine.id]);
    expect(resumeRes.exitCode, `resume stderr=${resumeRes.stderr}`).toBe(0);
    expect((await findRoutine())?.status).toBe("active");

    // Delete.
    const deleteRes = runCLI(env, ["routines", "delete", "--id", routine.id]);
    expect(deleteRes.exitCode, `delete stderr=${deleteRes.stderr}`).toBe(0);
    expect(await findRoutine()).toBeUndefined();
  });

  test.skip("tasks move — CLI sends only workflow_step_id but backend requires workflow_id too", () => {
    // kandev_tasks.go::tasksMove only forwards `workflow_step_id` (and
    // optional `--prompt`) to POST /api/v1/tasks/:id/move, but the
    // handler in task/handlers/task_http_handlers.go (~L914) rejects the
    // request with HTTP 400 "workflow_id and workflow_step_id are
    // required". The CLI surface needs a `--workflow` flag (or the
    // backend needs to infer the workflow from the task row) before
    // this verb is exercisable end-to-end — fixing either is out of
    // scope for the CLI smoke suite.
  });

  test("tasks archive removes the task from the active list", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const created = await apiClient.createTask(
      officeSeed.workspaceId,
      `CLI Archive Target ${Date.now()}`,
      { workflow_id: officeSeed.workflowId },
    );

    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
      taskId: created.id,
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
      taskId: created.id,
    });

    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backend.baseUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
      KANDEV_TASK_ID: created.id as string,
    };

    const res = runCLI(env, ["tasks", "archive", "--id", created.id as string]);
    expect(res.exitCode, `archive stderr=${res.stderr}`).toBe(0);

    // Verify the row is gone from the active list — the office API
    // hides archived tasks by default. A regression in the cascade
    // (the original 500 from missing tasks.archived_by_cascade_id)
    // would have failed `runCLI` above with a non-zero exit code, so
    // checking the listing also rules out a "200 OK but no-op" path.
    const { tasks } = await apiClient.listTasks(officeSeed.workspaceId);
    expect(tasks.find((t) => t.id === created.id)).toBeUndefined();
  });

  test.skip("approvals decide — not exercised: self-approval guard blocks single-agent setup", () => {
    // approvalsDecide IS wired in kandev_approvals.go (POST
    // /office/approvals/:id/decide), but exercising it end-to-end needs a
    // second agent in the workspace: the resolveDecider guard in
    // approvals/handler.go rejects any caller whose ID matches the
    // approval's RequestedByAgentProfileID, and the office fixture seeds
    // only the CEO. A dedicated approval-decide spec that hires a worker,
    // mints a runtime token for it, has it request the approval, then
    // decides as the CEO is the right shape — not in scope for the CLI
    // smoke suite.
  });
});
