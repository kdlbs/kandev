import { test, expect } from "../../fixtures/ssh-test-base";
import type { Page } from "@playwright/test";
import { execInContainer, killRemotePid, readRemoteFile } from "../../helpers/ssh";
import { dwell } from "../../helpers/causal-waits";
import { waitForLatestSessionDone, waitForSessionDone } from "../../helpers/session";
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
import {
  brokerAgentctlEnvironmentKeys,
  isRemoteProcessAlive,
  readSSHLaunchReceipt,
  remoteAgentctlEnvironmentKeys,
  remoteProcessTree,
  rewriteSSHReceiptForPreviousBoot,
  setupBrokerBackedSSH,
  sshSessionControllerPID,
  writeRemoteWorkspaceSentinel,
  type SSHReceiptState,
} from "../../helpers/ssh-broker-recovery";

const brokerResponse = "This is a simple mock response for e2e testing.";

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
    status: "ready",
  });
  expect(afterRow!.local_forward_port).toBeGreaterThan(0);
  execInContainer(seedData.sshTarget, ["kill", "-0", String(state.remotePid)]);
  const pidFile = `${state.beforeRow.remote_task_dir}/.kandev/sessions/${state.sessionId}/agentctl.pid`;
  expect(readRemoteFile(seedData.sshTarget, pidFile).trim()).toBe(String(state.remotePid));
}

type BrokerRecoveryState = {
  taskId: string;
  sessionId: string;
  session: SessionPage;
  executionId: string;
  receipt: SSHReceiptState;
  workspaceSentinel: string;
  siblingSessionId: string;
  siblingExecutionId: string;
  siblingReceipt: SSHReceiptState;
  siblingWorkspaceSentinel: string;
  snapshot: SilentRestartSnapshot;
};

async function prepareBrokerRecoveryState(
  page: Page,
  apiClient: ApiClient,
  seedData: SSHSeedData,
  backend: BackendContext,
  repositoryId: string,
): Promise<BrokerRecoveryState> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Broker-backed SSH resume recovery",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [repositoryId],
      executor_profile_id: seedData.sshExecutorProfileId,
    },
  );
  if (!task.session_id)
    throw new Error("broker-backed SSH task did not create its primary session");
  await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for broker-backed SSH launch");

  await page.goto(`/t/${task.id}?sessionId=${task.session_id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();

  const snapshot = await captureSilentRestartSnapshot(apiClient, task.id, task.session_id);
  const original = (await apiClient.getTaskSession(task.session_id)).session;
  const executionId = original.agent_execution_id;
  const workspacePath = original.workspace_path;
  if (!executionId || !workspacePath) {
    throw new Error("broker-backed SSH session is missing its execution or workspace identity");
  }
  const rows = await apiClient.listSSHSessions(seedData.sshExecutorId);
  const remote = rows.find((candidate) => candidate.session_id === task.session_id);
  if (!remote?.remote_task_dir)
    throw new Error("broker-backed SSH session has no remote task directory");
  const receipt = readSSHLaunchReceipt(backend, task.session_id, executionId);
  const controllerPID = sshSessionControllerPID(
    seedData.sshTarget,
    remote.remote_task_dir,
    task.session_id,
  );
  expect(controllerPID).toBe(receipt.pid);
  expect(remoteAgentctlEnvironmentKeys(seedData.sshTarget, controllerPID)).toEqual(
    brokerAgentctlEnvironmentKeys,
  );

  const workspaceSentinel = writeRemoteWorkspaceSentinel(
    seedData.sshTarget,
    workspacePath,
    `.kandev-ssh-resume-${task.session_id}.txt`,
    `preserved main workspace ${task.session_id}\n`,
  );

  const sibling = await apiClient.launchSession({
    task_id: task.id,
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.sshExecutorProfileId,
    workflow_step_id: seedData.startStepId,
    prompt: "/e2e:simple-message",
  });
  await waitForSessionDone(apiClient, task.id, sibling.session_id, "Wait for sibling SSH session");
  const siblingSession = (await apiClient.getTaskSession(sibling.session_id)).session;
  const siblingExecutionId = siblingSession.agent_execution_id;
  const siblingWorkspacePath = siblingSession.workspace_path;
  if (!siblingExecutionId || !siblingWorkspacePath) {
    throw new Error("sibling SSH session is missing its execution or workspace identity");
  }
  const siblingRemote = (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
    (candidate) => candidate.session_id === sibling.session_id,
  );
  if (!siblingRemote?.remote_task_dir) {
    throw new Error("sibling SSH session has no remote task directory");
  }
  const siblingReceipt = readSSHLaunchReceipt(backend, sibling.session_id, siblingExecutionId);
  expect(remoteAgentctlEnvironmentKeys(seedData.sshTarget, siblingReceipt.pid)).toEqual(
    brokerAgentctlEnvironmentKeys,
  );
  const siblingWorkspaceSentinel = writeRemoteWorkspaceSentinel(
    seedData.sshTarget,
    siblingWorkspacePath,
    `.kandev-ssh-resume-sibling-${sibling.session_id}.txt`,
    `preserved sibling workspace ${sibling.session_id}\n`,
  );

  return {
    taskId: task.id,
    sessionId: task.session_id,
    session,
    executionId,
    receipt,
    workspaceSentinel,
    siblingSessionId: sibling.session_id,
    siblingExecutionId,
    siblingReceipt,
    siblingWorkspaceSentinel,
    snapshot,
  };
}

async function recoverBrokerSSHSession(
  apiClient: ApiClient,
  state: BrokerRecoveryState,
  action: "resume" | "fresh_start",
): Promise<void> {
  const result = await apiClient.wsRequest<{ session_id: string }>(
    "session.recover",
    { task_id: state.taskId, session_id: state.sessionId, action },
    120_000,
  );
  expect(result.session_id).toBe(state.sessionId);
  await expect
    .poll(async () => (await apiClient.getTaskSession(state.sessionId)).session.state, {
      timeout: 60_000,
      message: `SSH ${action} did not return the session to input-ready state`,
    })
    .toBe("WAITING_FOR_INPUT");
}

async function assertBrokerSSHRecoveryState({
  page,
  apiClient,
  seedData,
  backend,
  state,
  action,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SSHSeedData;
  backend: BackendContext;
  state: BrokerRecoveryState;
  action: "resume" | "fresh_start";
}): Promise<void> {
  await page.reload();
  await state.session.waitForLoad();
  const recovered = (await apiClient.getTaskSession(state.sessionId)).session;
  expect(recovered.id).toBe(state.sessionId);
  expect(recovered.task_environment_id).toBe(state.snapshot.taskEnvironmentId);
  const recoveredNativeID = nativeSessionID(recovered.metadata);
  if (action === "resume") {
    expect(recoveredNativeID).toBe(state.snapshot.nativeSessionId);
  } else {
    expect(recoveredNativeID).not.toBe(state.snapshot.nativeSessionId);
  }

  const messages = (await apiClient.listSessionMessages(state.sessionId)).messages;
  for (const messageId of state.snapshot.messageIds) {
    expect(messages.map((message) => message.id)).toContain(messageId);
  }

  const currentExecutionId = recovered.agent_execution_id;
  if (!currentExecutionId)
    throw new Error("recovered SSH session has no active execution identity");
  const receipt = readSSHLaunchReceipt(backend, state.sessionId, currentExecutionId);
  expect(receipt.generation).not.toBe(state.receipt.generation);
  expect(receipt.pid).not.toBe(state.receipt.pid);
  expect(isRemoteProcessAlive(seedData.sshTarget, receipt.pid)).toBe(true);
  expect(remoteAgentctlEnvironmentKeys(seedData.sshTarget, receipt.pid)).toEqual(
    brokerAgentctlEnvironmentKeys,
  );
  expect(readRemoteFile(seedData.sshTarget, state.workspaceSentinel)).toBe(
    `preserved main workspace ${state.sessionId}\n`,
  );

  const sibling = (await apiClient.getTaskSession(state.siblingSessionId)).session;
  expect(sibling.agent_execution_id).toBe(state.siblingExecutionId);
  const siblingReceipt = readSSHLaunchReceipt(
    backend,
    state.siblingSessionId,
    state.siblingExecutionId,
  );
  expect(siblingReceipt.generation).toBe(state.siblingReceipt.generation);
  expect(siblingReceipt.pid).toBe(state.siblingReceipt.pid);
  expect(isRemoteProcessAlive(seedData.sshTarget, siblingReceipt.pid)).toBe(true);
  expect(readRemoteFile(seedData.sshTarget, state.siblingWorkspaceSentinel)).toBe(
    `preserved sibling workspace ${state.siblingSessionId}\n`,
  );

  const previousResponses = messages.filter(
    (message) => message.author_type === "agent" && message.content.includes(brokerResponse),
  ).length;
  const previousResponseIDs = new Set(
    messages
      .filter(
        (message) => message.author_type === "agent" && message.content.includes(brokerResponse),
      )
      .map((message) => message.id),
  );
  await state.session.sendMessage("/e2e:simple-message");
  await waitForSessionDone(
    apiClient,
    state.taskId,
    state.sessionId,
    "Wait for a response after SSH recovery",
  );
  await expect
    .poll(
      async () =>
        (await apiClient.listSessionMessages(state.sessionId)).messages.filter(
          (message) => message.author_type === "agent" && message.content.includes(brokerResponse),
        ).length,
      { timeout: 45_000, message: "the recovered controller did not accept a new prompt" },
    )
    .toBe(previousResponses + 1);
  const newResponses = (await apiClient.listSessionMessages(state.sessionId)).messages.filter(
    (message) =>
      message.author_type === "agent" &&
      message.content.includes(brokerResponse) &&
      !previousResponseIDs.has(message.id),
  );
  expect(newResponses).toHaveLength(1);
  const newResponseMessage = newResponses[0];
  if (!newResponseMessage) throw new Error("the recovered controller did not add a new response");
  const newResponse = state.session
    .activeChat()
    .locator(`[data-agent-message-body][data-message-id="${newResponseMessage.id}"]`);
  await expect(newResponse).toBeVisible({ timeout: 30_000 });
  await expect(newResponse).toContainText(brokerResponse);
}

function nativeSessionID(metadata: Record<string, unknown> | null | undefined): string {
  const acp = metadata?.acp;
  const sessionID =
    acp && typeof acp === "object" ? (acp as Record<string, unknown>).session_id : undefined;
  if (typeof sessionID !== "string" || sessionID.length === 0) {
    throw new Error("SSH session has no native conversation identity");
  }
  return sessionID;
}

/**
 * Backend-restart recovery. Persisted ExecutorRunning metadata
 * (ssh_host, ssh_remote_agentctl_pid, ssh_remote_agentctl_port, etc.) drives
 * ResumeRemoteInstance. It verifies receipt-backed controller identity before
 * adoption, uses `kill -0` for legacy rows, re-forwards the remote port, and
 * reattaches the stream manager.
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
    const unavailableTarget = {
      ...seedData.sshTarget,
      containerName: `${seedData.sshTarget.containerName}-missing`,
    };
    expect(() => isRemoteProcessAlive(unavailableTarget, 4242)).toThrow(/docker exec .* failed/);

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

test.describe("ssh executor — broker-backed resume recovery", () => {
  test.skip(
    !process.env.KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL?.trim(),
    "requires the isolated HTTPS credential broker fixture reachable from SSH",
  );

  test("agent.stop followed by resume starts a new broker-backed controller", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(300_000);
    const broker = await setupBrokerBackedSSH(backend, apiClient, seedData);
    try {
      const state = await prepareBrokerRecoveryState(
        testPage,
        apiClient,
        seedData,
        backend,
        broker.repositoryId,
      );
      const stopped = await apiClient.wsRequest<{ success: boolean }>("agent.stop", {
        agent_id: state.executionId,
      });
      expect(stopped.success).toBe(true);
      await expect
        .poll(() => isRemoteProcessAlive(seedData.sshTarget, state.receipt.pid), {
          timeout: 30_000,
          message: "agent.stop did not terminate its owned SSH controller",
        })
        .toBe(false);

      await recoverBrokerSSHSession(apiClient, state, "resume");
      await assertBrokerSSHRecoveryState({
        page: testPage,
        apiClient,
        seedData,
        backend,
        state,
        action: "resume",
      });
    } finally {
      await broker.cleanup();
    }
  });

  test("external SIGTERM followed by resume replaces the controller and its child", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(300_000);
    const broker = await setupBrokerBackedSSH(backend, apiClient, seedData);
    try {
      const state = await prepareBrokerRecoveryState(
        testPage,
        apiClient,
        seedData,
        backend,
        broker.repositoryId,
      );
      const controllerChildren = remoteProcessTree(seedData.sshTarget, state.receipt.pid);
      expect(controllerChildren.length).toBeGreaterThan(0);
      execInContainer(seedData.sshTarget, ["kill", "-TERM", String(state.receipt.pid)]);
      await expect
        .poll(
          () =>
            [state.receipt.pid, ...controllerChildren].filter((pid) =>
              isRemoteProcessAlive(seedData.sshTarget, pid),
            ),
          { timeout: 30_000, message: "SIGTERM left the SSH controller or child process alive" },
        )
        .toEqual([]);

      await recoverBrokerSSHSession(apiClient, state, "resume");
      await assertBrokerSSHRecoveryState({
        page: testPage,
        apiClient,
        seedData,
        backend,
        state,
        action: "resume",
      });
    } finally {
      await broker.cleanup();
    }
  });

  test("a synthetic previous-boot receipt resumes through the broker", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(360_000);
    const broker = await setupBrokerBackedSSH(backend, apiClient, seedData);
    try {
      const state = await prepareBrokerRecoveryState(
        testPage,
        apiClient,
        seedData,
        backend,
        broker.repositoryId,
      );
      await simulateSyntheticReboot(backend, seedData, state);
      await recoverBrokerSSHSession(apiClient, state, "resume");
      await assertBrokerSSHRecoveryState({
        page: testPage,
        apiClient,
        seedData,
        backend,
        state,
        action: "resume",
      });
    } finally {
      await backend.ensureReady();
      await broker.cleanup();
    }
  });

  test("a synthetic previous-boot receipt supports fresh_start", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(360_000);
    const broker = await setupBrokerBackedSSH(backend, apiClient, seedData);
    try {
      const state = await prepareBrokerRecoveryState(
        testPage,
        apiClient,
        seedData,
        backend,
        broker.repositoryId,
      );
      await simulateSyntheticReboot(backend, seedData, state);
      await recoverBrokerSSHSession(apiClient, state, "fresh_start");
      await assertBrokerSSHRecoveryState({
        page: testPage,
        apiClient,
        seedData,
        backend,
        state,
        action: "fresh_start",
      });
    } finally {
      await backend.ensureReady();
      await broker.cleanup();
    }
  });
});

async function simulateSyntheticReboot(
  backend: BackendContext,
  seedData: SSHSeedData,
  state: BrokerRecoveryState,
): Promise<void> {
  const controllerChildren = remoteProcessTree(seedData.sshTarget, state.receipt.pid);
  execInContainer(seedData.sshTarget, ["kill", "-TERM", String(state.receipt.pid)]);
  await expect
    .poll(
      () =>
        [state.receipt.pid, ...controllerChildren].filter((pid) =>
          isRemoteProcessAlive(seedData.sshTarget, pid),
        ),
      {
        timeout: 30_000,
        message: "fixture-owned SSH controller or child did not exit before the synthetic reboot",
      },
    )
    .toEqual([]);

  const backendPID = backend.pid();
  expect(backendPID).toBeTruthy();
  process.kill(-backendPID!, "SIGKILL");
  await expectBackendOffline(backend);
  await expect
    .poll(() => isLocalProcessAlive(backendPID!), {
      timeout: 30_000,
      message: "fixture backend did not exit before its SQLite receipt update",
    })
    .toBe(false);

  const changedBirthToken = rewriteSSHReceiptForPreviousBoot(
    backend,
    state.sessionId,
    state.executionId,
  );
  expect(changedBirthToken).not.toBe(state.receipt.birthToken);
  await backend.restart();
}

function isLocalProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ESRCH") return false;
    throw error;
  }
}
