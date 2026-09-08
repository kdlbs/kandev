import { existsSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect } from "@playwright/test";
import { restoreSeedRepositoryOrigin, test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";

/**
 * The dev script runs as a real OS process, so these assertions read the OS
 * rather than the DOM: the dev script writes a pid to a file and the spec reads
 * that pid's state. Playwright and the backend share a process namespace in
 * both host and container runs, so the pid is meaningful here.
 *
 * Guards the lifecycle contract from #2723: a dev server started from the task
 * UI must not outlive the task that started it.
 *
 * `kill(pid, 0)` is deliberately NOT the check. It reports existence, not
 * liveness, and a killed process stays in the table as a zombie until its
 * parent reaps it. A dev script's background child is reaped by init on a
 * developer's machine but not inside the CI container, whose PID 1 does not
 * reap orphans — so signal 0 called that child alive for a full minute after
 * Kandev had killed it, and only in CI. The direct process never showed it,
 * because agentctl's own `cmd.Wait()` reaps that one.
 */
const HAS_PROCFS = existsSync("/proc/self/stat");

type ProcessState = "gone" | "zombie" | { running: string };

function processState(pid: number): ProcessState {
  if (!HAS_PROCFS) {
    // Non-Linux fallback. Its orphans are reaped by launchd/init, so the
    // zombie window this guards against does not persist there.
    try {
      process.kill(pid, 0);
      return { running: "unknown" };
    } catch {
      return "gone";
    }
  }
  let stat: string;
  try {
    stat = readFileSync(`/proc/${pid}/stat`, "utf8");
  } catch {
    return "gone";
  }
  // `comm` is parenthesised and may itself contain spaces and parens, so the
  // state field is located from the LAST ")" rather than by splitting.
  const state = stat
    .slice(stat.lastIndexOf(")") + 2)
    .trim()
    .split(/\s+/)[0];
  return state === "Z" ? "zombie" : { running: state };
}

function isRunning(pid: number): boolean {
  return typeof processState(pid) === "object";
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
  backendTmpDir: string,
  title: string,
  devScript: string,
) {
  // Worktree preparation reads the shared seed checkout. Restore both its
  // remote and working tree so a preceding source-mutating scenario cannot
  // leave this launch with a dirty index or detached branch.
  restoreSeedRepositoryOrigin(seedData);
  const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backendTmpDir));
  git.exec("git reset --hard origin/main");
  git.exec("git clean -fdx");

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

  // The session can be created before the executor has prepared its workspace.
  // Starting the process before that preparation completes is a real race, not
  // a reason to retry the test.
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        if (environment?.status === "failed") {
          throw new Error("dev process seed environment failed before it became ready");
        }
        return environment?.status ?? "missing";
      },
      {
        timeout: 90_000,
        intervals: [500, 1000, 2000],
        message: "dev process seed environment never became ready",
      },
    )
    .toBe("ready");

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
      { timeout: 90_000, intervals: [1000, 2000], message: "dev process never started" },
    )
    .toBe(200);
}

async function expectProcessReaped(pid: number): Promise<void> {
  // Reports the observed state, so a genuine failure says which process is
  // still scheduled rather than only that the pid is still in the table.
  await expect
    .poll(() => JSON.stringify(processState(pid)), {
      timeout: 60_000,
      message: `dev process ${pid} still running after its task ended`,
    })
    .toMatch(/"gone"|"zombie"/);
}

test.describe("dev server process lifecycle", () => {
  // Each test creates a task, launches an agent, prepares a worktree, starts a
  // real process, and then waits on asynchronous backend cleanup. That is well
  // past the 60s default: under it the polls below can never spend the budget
  // they ask for, so a loaded CI runner fails on the test deadline instead of
  // on the thing being asserted.
  test.describe.configure({ timeout: 240_000 });

  test("archiving the task stops the dev script, including its background children", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    const parentPidFile = pidFilePath("kandev-dev-archive-parent-");
    const childPidFile = pidFilePath("kandev-dev-archive-child-");
    const { taskId, sessionId } = await seedDevScriptTask(
      apiClient,
      seedData,
      backend.tmpDir,
      "Dev server archive",
      // `sleep &` stands in for the worker a real dev server forks: it must die
      // with its parent, which only holds if the whole process group is reaped.
      `sleep 600 & echo $! > ${childPidFile}; echo $$ > ${parentPidFile}; wait`,
    );

    await startDevProcess(apiClient, sessionId);
    const parentPid = await readPidWhenWritten(parentPidFile);
    const childPid = await readPidWhenWritten(childPidFile);
    expect(isRunning(parentPid)).toBe(true);
    expect(isRunning(childPid)).toBe(true);

    await apiClient.archiveTask(taskId);

    await expectProcessReaped(parentPid);
    await expectProcessReaped(childPid);
  });

  test("deleting the task stops the dev script", async ({ apiClient, backend, seedData }) => {
    const pidFile = pidFilePath("kandev-dev-delete-");
    const { taskId, sessionId } = await seedDevScriptTask(
      apiClient,
      seedData,
      backend.tmpDir,
      "Dev server delete",
      `echo $$ > ${pidFile}; exec sleep 600`,
    );

    await startDevProcess(apiClient, sessionId);
    const pid = await readPidWhenWritten(pidFile);
    expect(isRunning(pid)).toBe(true);

    await apiClient.deleteTask(taskId);

    await expectProcessReaped(pid);
  });

  test("the header control stops the dev script without archiving the task", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    const parentPidFile = pidFilePath("kandev-dev-stop-parent-");
    const childPidFile = pidFilePath("kandev-dev-stop-child-");
    const { sessionId } = await seedDevScriptTask(
      apiClient,
      seedData,
      backend.tmpDir,
      "Dev server manual stop",
      `sleep 600 & echo $! > ${childPidFile}; echo $$ > ${parentPidFile}; wait`,
    );

    await startDevProcess(apiClient, sessionId);
    const parentPid = await readPidWhenWritten(parentPidFile);
    const childPid = await readPidWhenWritten(childPidFile);
    expect(isRunning(parentPid)).toBe(true);
    expect(isRunning(childPid)).toBe(true);

    const processes = (await (
      await apiClient.rawRequest("GET", `/api/v1/task-sessions/${sessionId}/processes`)
    ).json()) as Array<{ id: string; kind: string }>;
    const devProcess = processes.find((process) => process.kind === "dev");
    expect(devProcess).toBeDefined();

    await apiClient.rawRequest(
      "POST",
      `/api/v1/task-sessions/${sessionId}/processes/${devProcess!.id}/stop`,
    );

    await expectProcessReaped(parentPid);
    await expectProcessReaped(childPid);
  });
});
