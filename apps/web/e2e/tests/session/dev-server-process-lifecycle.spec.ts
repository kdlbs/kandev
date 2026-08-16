import { existsSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

/**
 * The dev script runs as a real OS process, so these assertions read the OS
 * rather than the DOM: the dev script writes a pid to a file and the spec
 * probes that pid with signal 0. Playwright and the backend share a process
 * namespace in both host and container runs, so the pid is meaningful here.
 *
 * Guards the lifecycle contract from #2723: a dev server started from the task
 * UI must not outlive the task that started it.
 */
function isAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function pidFilePath(prefix: string): string {
  return join(mkdtempSync(join(tmpdir(), prefix)), "pid");
}

async function readPidWhenWritten(file: string): Promise<number> {
  let pid = 0;
  await expect
    .poll(
      () => {
        if (!existsSync(file)) return 0;
        pid = Number(readFileSync(file, "utf8").trim());
        return Number.isFinite(pid) ? pid : 0;
      },
      { timeout: 60_000, message: `dev script never wrote its pid to ${file}` },
    )
    .toBeGreaterThan(0);
  return pid;
}

async function seedDevScriptTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  devScript: string,
) {
  await apiClient.updateRepository(seedData.repositoryId, { dev_script: devScript });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  return { taskId: task.id, sessionId: task.session_id };
}

/**
 * Starting the dev process needs the session's workspace to exist, which the
 * launch prepares asynchronously — the retry is on that preparation, not on a
 * UI shadow.
 */
async function startDevProcess(apiClient: ApiClient, sessionId: string): Promise<void> {
  await expect
    .poll(
      async () => {
        const res = await apiClient.rawRequest(
          "POST",
          `/api/v1/task-sessions/${sessionId}/processes/start`,
          { kind: "dev" },
        );
        return res.status;
      },
      { timeout: 120_000, intervals: [1000, 2000], message: "dev process never started" },
    )
    .toBe(200);
}

async function expectProcessReaped(pid: number): Promise<void> {
  await expect
    .poll(() => isAlive(pid), {
      timeout: 60_000,
      message: `dev process ${pid} outlived its task`,
    })
    .toBe(false);
}

test.describe("dev server process lifecycle", () => {
  test("archiving the task stops the dev script, including its background children", async ({
    apiClient,
    seedData,
  }) => {
    const parentPidFile = pidFilePath("kandev-dev-archive-parent-");
    const childPidFile = pidFilePath("kandev-dev-archive-child-");
    const { taskId, sessionId } = await seedDevScriptTask(
      apiClient,
      seedData,
      "Dev server archive",
      // `sleep &` stands in for the worker a real dev server forks: it must die
      // with its parent, which only holds if the whole process group is reaped.
      `sleep 600 & echo $! > ${childPidFile}; echo $$ > ${parentPidFile}; wait`,
    );

    await startDevProcess(apiClient, sessionId);
    const parentPid = await readPidWhenWritten(parentPidFile);
    const childPid = await readPidWhenWritten(childPidFile);
    expect(isAlive(parentPid)).toBe(true);
    expect(isAlive(childPid)).toBe(true);

    await apiClient.archiveTask(taskId);

    await expectProcessReaped(parentPid);
    await expectProcessReaped(childPid);
  });

  test("deleting the task stops the dev script", async ({ apiClient, seedData }) => {
    const pidFile = pidFilePath("kandev-dev-delete-");
    const { taskId, sessionId } = await seedDevScriptTask(
      apiClient,
      seedData,
      "Dev server delete",
      `echo $$ > ${pidFile}; exec sleep 600`,
    );

    await startDevProcess(apiClient, sessionId);
    const pid = await readPidWhenWritten(pidFile);
    expect(isAlive(pid)).toBe(true);

    await apiClient.deleteTask(taskId);

    await expectProcessReaped(pid);
  });

  test("the header control stops the dev script without archiving the task", async ({
    apiClient,
    seedData,
  }) => {
    const pidFile = pidFilePath("kandev-dev-stop-");
    const { sessionId } = await seedDevScriptTask(
      apiClient,
      seedData,
      "Dev server manual stop",
      `echo $$ > ${pidFile}; exec sleep 600`,
    );

    await startDevProcess(apiClient, sessionId);
    const pid = await readPidWhenWritten(pidFile);
    expect(isAlive(pid)).toBe(true);

    const processes = (await (
      await apiClient.rawRequest("GET", `/api/v1/task-sessions/${sessionId}/processes`)
    ).json()) as Array<{ id: string; kind: string }>;
    const devProcess = processes.find((process) => process.kind === "dev");
    expect(devProcess).toBeDefined();

    await apiClient.rawRequest(
      "POST",
      `/api/v1/task-sessions/${sessionId}/processes/${devProcess!.id}/stop`,
    );

    await expectProcessReaped(pid);
  });
});
