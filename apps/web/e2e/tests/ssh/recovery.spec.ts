import { test, expect } from "../../fixtures/ssh-test-base";
import type { Page } from "@playwright/test";
import { execInContainer, killRemotePid, readRemoteFile } from "../../helpers/ssh";
import { dwell } from "../../helpers/causal-waits";
import { waitForLatestSessionDone } from "../../helpers/session";
import { attachMessageAddCapture } from "../../helpers/ws-capture";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import {
  captureSilentRestartSnapshot,
  countSessionMessageAdds,
  expectRestartConversationIdentity,
  type SilentRestartSnapshot,
} from "../../helpers/silent-restart-recovery";
import { SessionPage } from "../../pages/session-page";
import type { BackendContext } from "../../fixtures/backend";
import type { SSHSeedData } from "../../fixtures/ssh-test-base";
import type { SSHSession } from "../../../lib/types/http-ssh";
import type { ApiClient } from "../../helpers/api-client";
import type { GatewayTrafficFrame } from "../../helpers/ws-traffic";
import type { MessageAddFrame } from "../../helpers/ws-capture";

type SlowSSHRestartState = {
  taskId: string;
  sessionId: string;
  session: SessionPage;
  snapshot: SilentRestartSnapshot;
  beforeRow: SSHSession;
  remotePid: number;
  messageAddsBeforeRestart: number;
};

async function prepareSlowSSHRestart(
  page: Page,
  apiClient: ApiClient,
  seedData: SSHSeedData,
  messageAdds: MessageAddFrame[],
): Promise<SlowSSHRestartState> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "SSH recovery replays output while backend is offline",
    seedData.agentProfileId,
    {
      description: "/slow 30",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.sshExecutorProfileId,
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toBeVisible({
    timeout: 30_000,
  });
  await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
    timeout: 30_000,
  });

  const sessionId = (await apiClient.listTaskSessions(task.id)).sessions.find(
    (candidate) => candidate.is_primary,
  )?.id;
  expect(sessionId).toBeTruthy();
  const snapshot = await captureSilentRestartSnapshot(apiClient, task.id, sessionId!);
  expect(snapshot.agentExecutionId).toBeTruthy();
  const rows = await apiClient.listSSHSessions(seedData.sshExecutorId);
  const beforeRow = rows.find((candidate) => candidate.task_id === task.id);
  expect(beforeRow?.remote_agentctl_port).toBeGreaterThan(0);
  expect(beforeRow?.remote_task_dir).toBeTruthy();
  const pidFile = `${beforeRow!.remote_task_dir}/.kandev/sessions/${sessionId}/agentctl.pid`;
  const remotePid = Number.parseInt(readRemoteFile(seedData.sshTarget, pidFile).trim(), 10);
  expect(remotePid).toBeGreaterThan(0);
  execInContainer(seedData.sshTarget, ["kill", "-0", String(remotePid)]);
  await expect(session.chat.getByText("Slow response complete", { exact: false })).toHaveCount(0);

  return {
    taskId: task.id,
    sessionId: sessionId!,
    session,
    snapshot,
    beforeRow: beforeRow!,
    remotePid,
    messageAddsBeforeRestart: countSessionMessageAdds(messageAdds, task.id, sessionId!),
  };
}

async function stopBackendWhileRemoteTurnCompletes(
  backend: BackendContext,
  seedData: SSHSeedData,
  remotePid: number,
): Promise<void> {
  const backendPid = backend.pid();
  expect(backendPid).toBeTruthy();
  process.kill(-backendPid!, "SIGKILL");
  await expectBackendOffline(backend);
  execInContainer(seedData.sshTarget, ["kill", "-0", String(remotePid)]);

  // The mock agent's /slow 30 timer advances independently on the SSH host.
  await dwell(
    31_000,
    "poll-interval",
    "the remote mock-agent /slow 30 prompt must finish while the backend is confirmed offline",
  );
  await expectBackendOffline(backend);
  execInContainer(seedData.sshTarget, ["kill", "-0", String(remotePid)]);
}

async function expectBackendOffline(backend: BackendContext): Promise<void> {
  await expect
    .poll(
      async () => {
        try {
          await fetch(`${backend.baseUrl}/ready`, {
            signal: AbortSignal.timeout(500),
          });
          return false;
        } catch {
          return true;
        }
      },
      { timeout: 10_000, message: "backend process group did not go offline" },
    )
    .toBe(true);
}

async function expectSlowSSHRestartRecovery({
  page,
  apiClient,
  seedData,
  backend,
  state,
  traffic,
  messageAdds,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SSHSeedData;
  backend: BackendContext;
  state: SlowSSHRestartState;
  traffic: GatewayTrafficFrame[];
  messageAdds: MessageAddFrame[];
}): Promise<void> {
  await backend.restart();
  await page.reload();
  await state.session.waitForLoad();
  await expectRestartConversationIdentity({
    page,
    api: apiClient,
    taskId: state.taskId,
    snapshot: state.snapshot,
    traffic,
    expectedState: "WAITING_FOR_INPUT",
    executionDisposition: "preserved",
  });
  expect(countSessionMessageAdds(messageAdds, state.taskId, state.sessionId)).toBe(
    state.messageAddsBeforeRestart,
  );
  const completedResponse = state.session.chat.getByText("Slow response complete", {
    exact: false,
  });
  await expect(completedResponse).toBeVisible({ timeout: 45_000 });
  await expect(completedResponse).toHaveCount(1);

  const afterRows = await apiClient.listSSHSessions(seedData.sshExecutorId);
  const afterRow = afterRows.find((candidate) => candidate.task_id === state.taskId);
  expect(afterRow).toMatchObject({
    session_id: state.beforeRow.session_id,
    remote_agentctl_port: state.beforeRow.remote_agentctl_port,
    status: "running",
  });
  expect(afterRow!.local_forward_port).toBeGreaterThan(0);
  const pidFile = `${state.beforeRow.remote_task_dir}/.kandev/sessions/${state.sessionId}/agentctl.pid`;
  expect(readRemoteFile(seedData.sshTarget, pidFile).trim()).toBe(String(state.remotePid));
}

/**
 * Backend-restart recovery. Persisted ExecutorRunning metadata
 * (ssh_host, ssh_remote_agentctl_pid, ssh_remote_agentctl_port, etc.) drives
 * ResumeRemoteInstance — re-dial the SSH connection, re-forward the recorded
 * remote port, verify `kill -0 pid`, and reattach the stream manager.
 *
 * Covers e2e-plan.md group J (J1–J4).
 */
test.describe("ssh executor — recovery after restart", () => {
  test("remote execution replays completed output after backend downtime with local survival disabled", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(300_000);
    const traffic = attachGatewayTrafficCapture(testPage);
    const messageAdds = attachMessageAddCapture(testPage);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_AGENT_SURVIVAL: "false",
    });

    try {
      const state = await prepareSlowSSHRestart(testPage, apiClient, seedData, messageAdds.frames);
      await stopBackendWhileRemoteTurnCompletes(backend, seedData, state.remotePid);
      await expectSlowSSHRestartRecovery({
        page: testPage,
        apiClient,
        seedData,
        backend,
        state,
        traffic: traffic.frames,
        messageAdds: messageAdds.frames,
      });
    } finally {
      await releaseFeature();
    }
  });

  test("live session survives a backend restart", async ({ apiClient, seedData, backend }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "J1 backend restart",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Pre-restart launch");

    const beforeSessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const beforeRow = beforeSessions.find((s) => s.task_id === task.id);
    expect(beforeRow).toBeDefined();
    const beforeRemotePort = beforeRow!.remote_agentctl_port;

    await backend.restart();

    await expect
      .poll(
        async () => {
          const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
          const row = sessions.find((s) => s.task_id === task.id);
          // Resume should land back on the same remote port (the agentctl
          // process is unchanged) but a *new* local forward port.
          return row && row.remote_agentctl_port === beforeRemotePort
            ? row.local_forward_port
            : null;
        },
        { timeout: 60_000 },
      )
      .toBeGreaterThan(0);
  });

  test("backend restart with a dead remote agentctl marks the execution stopped", async ({
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "J2 dead agentctl",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Launch before kill");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row).toBeDefined();
    const sessionDir = `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}`;
    const pidStr = readRemoteFile(seedData.sshTarget, `${sessionDir}/agentctl.pid`).trim();
    const pid = parseInt(pidStr, 10);
    expect(pid).toBeGreaterThan(0);

    killRemotePid(seedData.sshTarget, pid);

    await backend.restart();

    await expect
      .poll(
        async () => {
          const after = await apiClient.listSSHSessions(seedData.sshExecutorId);
          const r = after.find((s) => s.task_id === task.id);
          return r?.status ?? "absent";
        },
        { timeout: 60_000 },
      )
      .not.toBe("running");
  });

  test("persisted metadata keys are present in ExecutorRunning after launch", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "J4 persisted metadata",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Launch for metadata");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row?.host).toBe(seedData.sshTarget.host);
    expect(row?.user).toBe(seedData.sshTarget.user);
    expect(row?.remote_agentctl_port).toBeGreaterThan(0);
    expect(row?.local_forward_port).toBeGreaterThan(0);
    expect(row?.remote_task_dir).toMatch(/\/tasks\//);
  });
});
