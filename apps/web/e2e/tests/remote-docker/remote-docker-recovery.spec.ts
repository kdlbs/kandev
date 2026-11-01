import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import { test, expect } from "../../fixtures/remote-docker-test-base";
import type { RemoteDockerSeedData } from "../../fixtures/remote-docker-test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForAgentMessage, waitForLatestSessionDone } from "../../helpers/session";

/**
 * Same-session recovery on a Docker daemon reached over SSH.
 *
 * The first session is stopped without destroying its container, then resumed
 * with a new execution ID. The resumed launch must reconcile the previous
 * agentctl journal owner before opening the same journal, reattach to the same
 * container, and keep work that exists only inside that container.
 */
function containerExists(containerId: string): boolean {
  const out = execFileSync("docker", ["ps", "-aq", "--no-trunc", "--filter", `id=${containerId}`], {
    encoding: "utf8",
  })
    .toString()
    .trim();
  return out !== "";
}

function execInContainer(containerId: string, script: string): string {
  return execFileSync("docker", ["exec", containerId, "sh", "-lc", script], {
    encoding: "utf8",
  })
    .toString()
    .trim();
}

function findRepositoryRoot(containerId: string): string {
  const root = execInContainer(
    containerId,
    "repo=$(find /workspace -maxdepth 3 -name .git | head -1); " +
      'if [ -n "$repo" ]; then dirname "$repo"; else echo /workspace; fi',
  );
  expect(root, "the task container has no writable workspace").toMatch(/^\/workspace/);
  return root;
}

function journalPath(sessionId: string): string {
  return `/var/lib/kandev/agentctl-journals/${sessionId}/delivery.bbolt`;
}

const journalLockReady = "/tmp/kandev-journal-lock-ready";
const journalLockWaiting = "/tmp/kandev-journal-lock-waiting";
const journalLockPipe = "/tmp/kandev-journal-lock-pipe";

function startJournalLock(containerId: string, path: string): ChildProcess {
  return spawn(
    "docker",
    [
      "exec",
      containerId,
      "sh",
      "-lc",
      `rm -f ${journalLockReady} ${journalLockWaiting} ${journalLockPipe}; ` +
        `mkfifo ${journalLockPipe}; touch ${journalLockWaiting}; ` +
        `flock -x ${path} -c 'touch ${journalLockReady}; read line < ${journalLockPipe}'`,
    ],
    { stdio: "ignore" },
  );
}

async function releaseJournalLock(containerId: string, holder: ChildProcess, path: string) {
  execInContainer(
    containerId,
    `timeout 5 sh -c "printf 'release\\n' > ${journalLockPipe}" || true`,
  );
  holder.kill();
  await expect
    .poll(
      () =>
        execInContainer(
          containerId,
          `flock -n ${path} -c true >/dev/null 2>&1 && echo free || echo locked`,
        ),
      { timeout: 30_000, message: "Wait for the external journal lock to release" },
    )
    .toBe("free");
}

async function prepareRecoverableTask(
  apiClient: ApiClient,
  seedData: RemoteDockerSeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.remoteDockerExecutorProfileId,
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for the remote Docker session");
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const sessionId = sessions[0]?.id;
  expect(sessionId, "the launched session is missing").toBeTruthy();

  const env = await apiClient.getTaskEnvironment(task.id);
  const containerId = env?.container_id;
  expect(containerId, "no container recorded for the task").toBeTruthy();
  expect(containerExists(containerId!)).toBe(true);

  // Work that exists only in the container: a committed marker and an
  // untracked marker. Neither is reachable from any backend checkout.
  const repoRoot = findRepositoryRoot(containerId!);
  const markerCommit = execInContainer(
    containerId!,
    `cd ${repoRoot} && ` +
      "if [ ! -d .git ] && [ ! -f .git ]; then git init -q; fi && " +
      "printf 'recovery marker\\n' > recovery-marker.txt && " +
      "git add recovery-marker.txt && " +
      "git -c user.name='E2E Test' -c user.email='e2e@test.local' commit -q -m 'recovery marker' && " +
      "git rev-parse HEAD",
  );
  expect(markerCommit).toMatch(/^[0-9a-f]{40}$/);
  execInContainer(
    containerId!,
    `cd ${repoRoot} && printf 'untracked marker\\n' > untracked-recovery-marker.txt`,
  );

  return { task, sessionId: sessionId!, containerId: containerId!, repoRoot, markerCommit };
}

async function stopSessionAndWait(apiClient: ApiClient, taskId: string, sessionId: string) {
  await apiClient.stopSession({ session_id: sessionId });
  await expect
    .poll(async () => (await apiClient.listTaskSessions(taskId)).sessions[0]?.state, {
      timeout: 60_000,
      message: "Wait for the stopped session",
    })
    .toBe("CANCELLED");
}

async function waitForResumedSession(apiClient: ApiClient, taskId: string, sessionId: string) {
  await expect
    .poll(
      async () =>
        (await apiClient.listTaskSessions(taskId)).sessions.find(
          (session) => session.id === sessionId,
        )?.state,
      { timeout: 90_000, message: "Wait for the resumed session to become ready" },
    )
    .toBe("WAITING_FOR_INPUT");
  const { session } = await apiClient.getTaskSession(sessionId);
  expect(
    session.session_recovery_blocks ?? [],
    "a recovered session must have no current block",
  ).toEqual([]);
}

async function sendMarkerTurn(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string> {
  const marker = `resume-marker-${randomUUID()}`;
  await apiClient.addUserMessage(taskId, sessionId, `e2e:message("${marker}")`);
  await waitForAgentMessage(apiClient, sessionId, marker, 120_000);
  return marker;
}

test.describe("remote docker executor — recoverable resume", () => {
  test("same-session resume keeps the container, journal, and unpushed work", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(300_000);
    const { task, sessionId, containerId, repoRoot, markerCommit } = await prepareRecoverableTask(
      apiClient,
      seedData,
      "remote docker recoverable resume",
    );

    await stopSessionAndWait(apiClient, task.id, sessionId);
    const resumed = await apiClient.launchSession(
      {
        task_id: task.id,
        session_id: sessionId,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.remoteDockerExecutorProfileId,
        intent: "resume",
      },
      90_000,
    );
    expect(resumed.session_id).toBe(sessionId);
    expect(resumed.agent_execution_id).toMatch(/\S/);
    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(task.id)).sessions.find(
            (session) => session.id === sessionId,
          )?.agent_execution_id,
        { timeout: 60_000, message: "Wait for the replacement execution identity" },
      )
      .toBe(resumed.agent_execution_id);
    await waitForResumedSession(apiClient, task.id, sessionId);
    await sendMarkerTurn(apiClient, task.id, sessionId);

    const resumedEnv = await apiClient.getTaskEnvironment(task.id);
    expect(resumedEnv?.container_id, "resume launched a new container").toBe(containerId);
    expect(containerExists(containerId)).toBe(true);
    expect(findRepositoryRoot(containerId)).toBe(repoRoot);
    expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(markerCommit);
    expect(execInContainer(containerId, `cat ${repoRoot}/recovery-marker.txt`)).toContain(
      "recovery marker",
    );
    expect(execInContainer(containerId, `cat ${repoRoot}/untracked-recovery-marker.txt`)).toContain(
      "untracked marker",
    );

    await apiClient.deleteTask(task.id);
    await expect
      .poll(() => containerExists(containerId), {
        message: "container survived task deletion on the remote daemon",
        timeout: 90_000,
      })
      .toBe(false);
  });

  test("blocked journal acquisition preserves the workspace and retries after release", async ({
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(300_000);
    const { task, sessionId, containerId, repoRoot, markerCommit } = await prepareRecoverableTask(
      apiClient,
      seedData,
      "remote docker blocked journal recovery",
    );

    await stopSessionAndWait(apiClient, task.id, sessionId);
    const path = journalPath(sessionId);
    const holder = startJournalLock(containerId, path);
    try {
      await expect
        .poll(
          () =>
            execInContainer(
              containerId,
              `test -f ${journalLockWaiting} && echo waiting || echo absent`,
            ),
          { timeout: 30_000, message: "Wait for the external journal lock to start" },
        )
        .toBe("waiting");

      const before = (await apiClient.getTaskSession(sessionId)).session.agent_execution_id;
      let launchError: unknown;
      let launchResult: { state?: string } | undefined;
      try {
        launchResult = await apiClient.launchSession(
          {
            task_id: task.id,
            session_id: sessionId,
            agent_profile_id: seedData.agentProfileId,
            executor_profile_id: seedData.remoteDockerExecutorProfileId,
            intent: "resume",
          },
          90_000,
        );
      } catch (error) {
        launchError = error;
      }
      expect(
        launchError !== undefined || launchResult?.state !== "WAITING_FOR_INPUT",
        "a held journal lock must not produce a ready session",
      ).toBe(true);

      const launchErrorText = String(launchError);
      expect(launchErrorText).toMatch(
        /delivery journal unavailable: open delivery journal: lock_timeout/i,
      );
      const backendLog = fs.readFileSync(backend.logPath, "utf8");
      const failedLaunchRecord = backendLog
        .split("\n")
        .find(
          (line) =>
            line.includes("failed to relaunch agent for session") &&
            line.includes(sessionId) &&
            line.includes("lock_timeout"),
        );
      expect(
        failedLaunchRecord,
        "the backend must persist the journal failure for this session",
      ).toBeTruthy();
      const blockedExecutionID = /instance ([0-9a-f-]{36}) delivery journal unavailable/.exec(
        failedLaunchRecord!,
      )?.[1];
      expect(blockedExecutionID, "the journal failure must name the blocked attempt").toBeTruthy();
      expect(blockedExecutionID).not.toBe(before ?? "");
      expect((await apiClient.getTaskSession(sessionId)).session.state).toBe("CANCELLED");

      await expect
        .poll(
          () =>
            execInContainer(
              containerId,
              `test -f ${journalLockReady} && echo ready || echo waiting`,
            ),
          { timeout: 60_000, message: "Wait for the blocked writer to acquire the released lock" },
        )
        .toBe("ready");

      expect(containerExists(containerId), "blocked recovery removed the workspace").toBe(true);
      expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(
        markerCommit,
      );
      expect(execInContainer(containerId, `cat ${repoRoot}/recovery-marker.txt`)).toContain(
        "recovery marker",
      );
    } finally {
      await releaseJournalLock(containerId, holder, path);
    }

    // The reported incident crossed a backend restart. The container, journal,
    // and durable execution record must survive it before the retry.
    await backend.restart();

    const resumed = await apiClient.launchSession(
      {
        task_id: task.id,
        session_id: sessionId,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.remoteDockerExecutorProfileId,
        intent: "resume",
      },
      90_000,
    );
    expect(resumed.session_id).toBe(sessionId);
    await waitForResumedSession(apiClient, task.id, sessionId);
    await sendMarkerTurn(apiClient, task.id, sessionId);
    expect((await apiClient.getTaskEnvironment(task.id))?.container_id).toBe(containerId);
    expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(markerCommit);
    expect(execInContainer(containerId, `cat ${repoRoot}/untracked-recovery-marker.txt`)).toContain(
      "untracked marker",
    );

    await apiClient.deleteTask(task.id);
    await expect
      .poll(() => containerExists(containerId), {
        message: "container survived task deletion on the remote daemon",
        timeout: 90_000,
      })
      .toBe(false);
  });
});
