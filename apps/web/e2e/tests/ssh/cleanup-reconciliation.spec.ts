import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/ssh-test-base";
import {
  execInContainer,
  killRemotePid,
  readRemoteFile,
  remotePathExists,
} from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import type { SSHSeedData } from "../../fixtures/ssh-test-base";

type SSHTaskController = {
  taskId: string;
  sessionId: string;
  sessionDir: string;
  taskDir: string;
  pid: number;
};

async function launchSSHTask(
  apiClient: ApiClient,
  seedData: SSHSeedData,
  title: string,
): Promise<SSHTaskController> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.sshExecutorProfileId,
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for the SSH controller launch");

  const row = (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
    (candidate) => candidate.task_id === task.id,
  );
  expect(row).toBeDefined();
  const sessionDir = `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}`;
  const pid = Number.parseInt(
    readRemoteFile(seedData.sshTarget, `${sessionDir}/agentctl.pid`).trim(),
    10,
  );
  expect(pid).toBeGreaterThan(0);
  execInContainer(seedData.sshTarget, ["kill", "-0", String(pid)]);

  return {
    taskId: task.id,
    sessionId: row!.session_id,
    sessionDir,
    taskDir: row!.remote_task_dir!,
    pid,
  };
}

function remotePidAlive(seedData: SSHSeedData, pid: number): boolean {
  try {
    execInContainer(seedData.sshTarget, ["kill", "-0", String(pid)]);
    return true;
  } catch {
    return false;
  }
}

function backendLogHasCleanupFailure(
  backend: BackendContext,
  taskId: string,
  reason: string,
): boolean {
  const paths = [path.join(backend.tmpDir, "backend-process.log"), backend.logPath];
  for (const logPath of paths) {
    try {
      const found = fs
        .readFileSync(logPath, "utf8")
        .split("\n")
        .some(
          (line) =>
            line.includes("task cleanup runtime stop failed") &&
            line.includes(taskId) &&
            line.includes(reason),
        );
      if (found) return true;
    } catch {
      // Try the other configured backend log.
    }
  }
  return false;
}

function removeFixtureController(seedData: SSHSeedData, controller: SSHTaskController): void {
  try {
    if (remotePidAlive(seedData, controller.pid)) {
      killRemotePid(seedData.sshTarget, controller.pid);
    }
  } catch {
    // The container may already be gone, or the controller already exited.
  }
  try {
    execInContainer(seedData.sshTarget, ["rm", "-rf", controller.sessionDir]);
  } catch {
    // Best-effort teardown of fixture-owned state.
  }
}

// AC-EXECUTORS-SSH-EXECUTOR-001.19, -001.22, -001.23, -001.24
test("reconciles the original controller after backend restart and session-directory loss", async ({
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(300_000);
  const target = await launchSSHTask(apiClient, seedData, "SSH directory loss reconciliation");
  const sibling = await launchSSHTask(apiClient, seedData, "SSH directory loss sibling");

  try {
    // Lose backend memory and the disposable session directory together. The
    // startup recovery pass cannot re-prove the controller, so the archive
    // reaches the persisted stop path.
    execInContainer(seedData.sshTarget, ["rm", "-rf", target.sessionDir]);
    await backend.restart();
    await backend.ensureReady();

    await apiClient.archiveTask(target.taskId);

    await expect
      .poll(() => remotePidAlive(seedData, target.pid), {
        timeout: 120_000,
        message: "Wait for directory-loss reconciliation to stop the original controller",
      })
      .toBe(false);
    // Settlement, not only process exit: the durable executor row disappears
    // once the exact target completes.
    await expect
      .poll(
        async () =>
          (await apiClient.listSSHSessions(seedData.sshExecutorId)).some(
            (session) => session.task_id === target.taskId,
          ),
        { timeout: 120_000, message: "Wait for exact-target cleanup settlement" },
      )
      .toBe(false);
    expect(remotePathExists(seedData.sshTarget, target.sessionDir)).toBe(false);
    expect(remotePathExists(seedData.sshTarget, target.taskDir)).toBe(true);
    // One completed target must not disturb a sibling controller.
    expect(remotePidAlive(seedData, sibling.pid)).toBe(true);
    expect(remotePathExists(seedData.sshTarget, sibling.taskDir)).toBe(true);

    await apiClient.archiveTask(sibling.taskId);
  } finally {
    removeFixtureController(seedData, target);
    removeFixtureController(seedData, sibling);
    await apiClient.archiveTask(sibling.taskId).catch(() => undefined);
  }
});

// AC-EXECUTORS-SSH-EXECUTOR-001.20, -001.23
test("preserves a replacement session directory and its controller", async ({
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(300_000);
  const target = await launchSSHTask(apiClient, seedData, "SSH replacement directory protected");
  const marker = `${target.sessionDir}/replacement-marker`;
  expect(target.sessionDir).toMatch(/^[A-Za-z0-9/._-]+$/);

  try {
    execInContainer(seedData.sshTarget, ["rm", "-rf", target.sessionDir]);
    execInContainer(seedData.sshTarget, [
      "sh",
      "-c",
      `mkdir -p '${target.sessionDir}' && printf replacement > '${marker}'`,
    ]);
    await backend.restart();
    await backend.ensureReady();

    await apiClient.archiveTask(target.taskId);

    // Wait for the durable, target-correlated refusal evidence before
    // asserting preservation. The archived_at mutation alone does not prove
    // the asynchronous runtime-stop attempt ran.
    await expect
      .poll(() => backendLogHasCleanupFailure(backend, target.taskId, "agentctl.pid"), {
        timeout: 120_000,
        message: "Wait for the replacement-directory cleanup refusal",
      })
      .toBe(true);

    // Cleanup keeps the replacement evidence and the recorded controller. The
    // unit-level owner tests prove the helper refused the signal; this spec
    // proves the refusal has no destructive side effect on the host.
    expect(remotePathExists(seedData.sshTarget, marker)).toBe(true);
    expect(readRemoteFile(seedData.sshTarget, marker)).toBe("replacement");
    expect(remotePidAlive(seedData, target.pid)).toBe(true);
  } finally {
    removeFixtureController(seedData, target);
  }
});
