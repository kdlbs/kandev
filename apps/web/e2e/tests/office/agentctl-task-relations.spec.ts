import path from "node:path";
import { spawnSync } from "node:child_process";
import { test, expect } from "../../fixtures/office-fixture";
import type { ApiClient } from "../../helpers/api-client";

/**
 * End-to-end coverage for `agentctl kandev task create --blocked-by` and
 * `--workspace-mode` (REQ-OFFICE-AGENT-TASK-RELATIONS-001/003/004). Each test
 * runs the real agentctl binary with a minted runtime token, exactly as an
 * Office run would, against the live backend.
 */

const BACKEND_DIR = path.resolve(__dirname, "../../../../../apps/backend");
const AGENTCTL_BIN = path.join(BACKEND_DIR, "bin", "agentctl");

type CLIResult = { exitCode: number; output: string; taskId?: string };

function runCLI(env: NodeJS.ProcessEnv, args: string[]): CLIResult {
  const proc = spawnSync(AGENTCTL_BIN, ["kandev", ...args], {
    env: { ...process.env, ...env },
    encoding: "utf8",
    timeout: 15_000,
  });
  const stdout = proc.stdout?.toString() ?? "";
  const output = stdout + (proc.stderr?.toString() ?? "");
  let taskId: string | undefined;
  try {
    taskId = (JSON.parse(stdout) as { task_id?: string }).task_id;
  } catch {
    taskId = undefined;
  }
  return { exitCode: proc.status ?? -1, output, taskId };
}

function workspaceMode(metadata: Record<string, unknown> | null | undefined): unknown {
  const workspace = metadata?.workspace as Record<string, unknown> | undefined;
  return workspace?.mode;
}

test.describe("agentctl kandev task create relations", () => {
  async function coordinatorEnv(
    apiClient: ApiClient,
    backendUrl: string,
    officeSeed: { workspaceId: string; workflowId: string; agentId: string },
  ) {
    const ownTask = await apiClient.createTask(
      officeSeed.workspaceId,
      `CLI Relations Coordinator ${Date.now()}`,
      { workflow_id: officeSeed.workflowId },
    );
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "task_assigned",
      status: "claimed",
      taskId: ownTask.id,
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: run.run_id,
      taskId: ownTask.id,
      capabilities: JSON.stringify({
        create_task: true,
        create_subtask: true,
        allowed_task_ids: [ownTask.id],
      }),
    });
    const env: NodeJS.ProcessEnv = {
      KANDEV_API_URL: backendUrl,
      KANDEV_API_KEY: token,
      KANDEV_WORKSPACE_ID: officeSeed.workspaceId,
      KANDEV_AGENT_ID: officeSeed.agentId,
      KANDEV_RUN_ID: run.run_id,
      KANDEV_TASK_ID: ownTask.id as string,
    };
    return { ownTaskId: ownTask.id as string, env };
  }

  test("a join task created with --blocked-by records an edge to every fanned-out child", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const { ownTaskId, env } = await coordinatorEnv(apiClient, backend.baseUrl, officeSeed);

    const childA = runCLI(env, ["task", "create", "--title", "Batch A", "--parent", ownTaskId]);
    expect(childA.exitCode, childA.output).toBe(0);
    const childB = runCLI(env, ["task", "create", "--title", "Batch B", "--parent", ownTaskId]);
    expect(childB.exitCode, childB.output).toBe(0);
    expect(childA.taskId && childB.taskId).toBeTruthy();

    const join = runCLI(env, [
      "task",
      "create",
      "--title",
      "Integrate batches",
      "--parent",
      ownTaskId,
      "--blocked-by",
      `${childA.taskId},${childB.taskId}`,
    ]);
    expect(join.exitCode, join.output).toBe(0);

    const projection = await apiClient.getTaskDependencies(join.taskId as string);
    const dependsOn = (projection.depends_on ?? []).map((ref) => ref.id).sort();
    expect(dependsOn).toEqual([childA.taskId, childB.taskId].sort());
    expect(projection.blocked).toBe(true);
  });

  test("a blocker the run cannot read is refused", async ({ apiClient, backend, officeSeed }) => {
    const { ownTaskId, env } = await coordinatorEnv(apiClient, backend.baseUrl, officeSeed);
    const unrelated = await apiClient.createTask(
      officeSeed.workspaceId,
      `CLI Relations Unrelated ${Date.now()}`,
      { workflow_id: officeSeed.workflowId },
    );

    const denied = runCLI(env, [
      "task",
      "create",
      "--title",
      "Should not exist",
      "--parent",
      ownTaskId,
      "--blocked-by",
      unrelated.id as string,
    ]);
    expect(denied.exitCode).toBe(1);
    expect(denied.output).toContain("is not a task this run may depend on");
    expect(denied.output).not.toContain(unrelated.id as string);
    expect(denied.taskId).toBeUndefined();
  });

  test("--workspace-mode new_workspace gives a child its own workspace; omitting it inherits", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const { ownTaskId, env } = await coordinatorEnv(apiClient, backend.baseUrl, officeSeed);

    const isolated = runCLI(env, [
      "task",
      "create",
      "--title",
      "Isolated batch",
      "--parent",
      ownTaskId,
      "--workspace-mode",
      "new_workspace",
    ]);
    expect(isolated.exitCode, isolated.output).toBe(0);
    const shared = runCLI(env, [
      "task",
      "create",
      "--title",
      "Shared batch",
      "--parent",
      ownTaskId,
    ]);
    expect(shared.exitCode, shared.output).toBe(0);

    const isolatedTask = await apiClient.getTask(isolated.taskId as string);
    expect(workspaceMode(isolatedTask.metadata)).toBe("new_workspace");
    const sharedTask = await apiClient.getTask(shared.taskId as string);
    expect(workspaceMode(sharedTask.metadata)).toBe("inherit_parent");
  });

  test("flags the runtime does not support fail before any request", async ({
    apiClient,
    backend,
    officeSeed,
  }) => {
    const { env } = await coordinatorEnv(apiClient, backend.baseUrl, officeSeed);
    for (const flag of [
      "--priority",
      "--workspace-group-id",
      "--default-child-workspace",
      "--default-child-ordering",
    ]) {
      const result = runCLI(env, ["task", "create", "--title", "Unsupported", flag, "x"]);
      expect(result.exitCode, `${flag}: ${result.output}`).toBe(1);
      expect(result.output).toContain(`${flag} is not supported by Office runtime task create`);
      expect(result.taskId).toBeUndefined();
    }
  });
});
