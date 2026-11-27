import { verifyAutomaticRestartReconciliation } from "../../helpers/restart-reconciliation";
import { test, expect } from "../../fixtures/test-base";
import { randomUUID } from "node:crypto";
import { readdirSync, readFileSync, readlinkSync } from "node:fs";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { DatabaseSync } from "../../helpers/node-sqlite";
import { attachMessageAddCapture } from "../../helpers/ws-capture";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import type { ApiClient, MockGitHubWorkspaceConnection } from "../../helpers/api-client";
import {
  captureSilentRestartSnapshot,
  countSessionMessageAdds,
  expectRestartConversationIdentity,
} from "../../helpers/silent-restart-recovery";
import { SessionPage } from "../../pages/session-page";
function survivalTurnGate(tmpDir: string) {
  const marker = join(tmpDir, `survival-turn-${randomUUID()}.release`);
  return {
    prompt: [
      'e2e:message("Running slow response")',
      `e2e:await_file(${JSON.stringify(marker)})`,
      'e2e:message("Slow response complete")',
    ].join("\n"),
    release: () => writeFile(marker, "release"),
  };
}

async function localExecutorProfileId(apiClient: {
  listExecutors: () => Promise<{
    executors: Array<{ type: string; profiles?: Array<{ id: string }> }>;
  }>;
}): Promise<string> {
  const { executors } = await apiClient.listExecutors();
  const executor = executors.find((candidate) => ["local", "local_pc"].includes(candidate.type));
  const profile = executor?.profiles?.[0];
  if (!profile) throw new Error("E2E survival test requires a local_pc executor profile");
  return profile.id;
}

type ManagedCredentialProcess = {
  identity: string;
  pid: number;
  hasLease: boolean;
  hasReissueCapability: boolean;
  reissueCapability: string;
};

type ManagedAgentctlIdentity = {
  executionId: string;
  agentctlPort: number;
};

type AgentctlProcessIdentity = { pid: number; identity: string };

type WorkspaceGitCredentialsBaseline = {
  connection: MockGitHubWorkspaceConnection | null;
  taskGitCredentialsMode: string;
};

async function captureWorkspaceGitCredentialsBaseline(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<WorkspaceGitCredentialsBaseline> {
  const connectionResponse = await apiClient.rawRequest(
    "GET",
    `/api/v1/github/connections?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  if (!connectionResponse.ok) {
    throw new Error(`workspace GitHub connection read failed (${connectionResponse.status})`);
  }
  const connectionStatus = (await connectionResponse.json()) as {
    automation?: MockGitHubWorkspaceConnection | null;
  };
  const automation = connectionStatus.automation;
  const connection = automation
    ? {
        source: automation.source,
        status: automation.status,
        github_host: automation.github_host,
        login: automation.login,
        installation_id: automation.installation_id,
        installation_account_login: automation.installation_account_login,
        installation_account_type: automation.installation_account_type,
        app_registration_id: automation.app_registration_id,
        capabilities: automation.capabilities,
      }
    : null;

  const settingsResponse = await apiClient.rawRequest(
    "GET",
    `/api/v1/github/workspace-settings?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  if (!settingsResponse.ok) {
    throw new Error(`workspace Git credential settings read failed (${settingsResponse.status})`);
  }
  const settings = (await settingsResponse.json()) as { task_git_credentials_mode?: string };
  if (!settings.task_git_credentials_mode) {
    throw new Error("workspace Git credential mode was not returned");
  }
  return { connection, taskGitCredentialsMode: settings.task_git_credentials_mode };
}

async function restoreWorkspaceGitCredentialsBaseline(
  apiClient: ApiClient,
  workspaceId: string,
  baseline: WorkspaceGitCredentialsBaseline,
): Promise<void> {
  if (baseline.connection) {
    await apiClient
      .mockGitHubSetWorkspaceConnection(workspaceId, baseline.connection)
      .catch(() => undefined);
  } else {
    await apiClient.mockGitHubDeleteWorkspaceConnection(workspaceId).catch(() => undefined);
  }
  await apiClient
    .rawRequest("PUT", "/api/v1/github/workspace-settings", {
      workspace_id: workspaceId,
      task_git_credentials_mode: baseline.taskGitCredentialsMode,
    })
    .catch(() => undefined);
}

function managedAgentctlIdentity(
  tmpDir: string,
  sessionId: string,
): ManagedAgentctlIdentity | undefined {
  const database = new DatabaseSync(join(tmpDir, "kandev.db"));
  try {
    database.exec("PRAGMA busy_timeout = 10000");
    const row = database
      .prepare(
        "SELECT agent_execution_id, agentctl_port FROM executors_running WHERE session_id = ?",
      )
      .get(sessionId) as { agent_execution_id?: string; agentctl_port?: number } | undefined;
    if (!row?.agent_execution_id || !row.agentctl_port) return undefined;
    return {
      executionId: row.agent_execution_id,
      agentctlPort: row.agentctl_port,
    };
  } finally {
    database.close();
  }
}

function agentctlListeningSocketInodes(port: number): Set<string> {
  const portHex = port.toString(16).toUpperCase().padStart(4, "0");
  const socketInodes = new Set<string>();
  for (const table of ["/proc/net/tcp", "/proc/net/tcp6"]) {
    try {
      for (const line of readFileSync(table, "utf8").split("\n").slice(1)) {
        const fields = line.trim().split(/\s+/);
        if (
          fields[1]?.split(":").at(-1)?.toUpperCase() === portHex &&
          fields[3] === "0A" &&
          fields[9]
        ) {
          socketInodes.add(fields[9]);
        }
      }
    } catch {
      // IPv4 and IPv6 tables are independently available.
    }
  }
  return socketInodes;
}

function processOwnsAgentctlListener(processPath: string, socketInodes: Set<string>): boolean {
  return readdirSync(`${processPath}/fd`).some((descriptor) => {
    const link = readlinkSync(`${processPath}/fd/${descriptor}`);
    const inode = link.match(/^socket:\[(\d+)\]$/)?.[1];
    return inode !== undefined && socketInodes.has(inode);
  });
}

function agentctlProcessIdentity(
  entry: string,
  socketInodes: Set<string>,
): AgentctlProcessIdentity | undefined {
  const processPath = `/proc/${entry}`;
  try {
    if (!processOwnsAgentctlListener(processPath, socketInodes)) return undefined;
    const stat = readFileSync(`${processPath}/stat`, "utf8");
    const fields = stat
      .slice(stat.lastIndexOf(")") + 2)
      .trim()
      .split(/\s+/);
    const startTime = fields[19];
    return startTime ? { pid: Number(entry), identity: `${entry}:${startTime}` } : undefined;
  } catch {
    // A process can exit between reading its file descriptors and stat.
    return undefined;
  }
}

function agentctlProcessForPort(port: number): AgentctlProcessIdentity | undefined {
  const socketInodes = agentctlListeningSocketInodes(port);
  if (socketInodes.size === 0) return undefined;
  const matches = readdirSync("/proc").flatMap((entry) => {
    if (!/^\d+$/.test(entry)) return [];
    const identity = agentctlProcessIdentity(entry, socketInodes);
    return identity ? [identity] : [];
  });
  return matches.length === 1 ? matches[0] : undefined;
}

function agentProcessIdentityWithinAgentctl(
  processPath: string,
  pid: string,
  agentctlPid: number,
): string | undefined {
  let currentPid = Number(pid);
  const visited = new Set<number>();
  let startTime: string | undefined;
  try {
    while (currentPid !== 1 && !visited.has(currentPid)) {
      if (currentPid === agentctlPid) {
        return startTime ? `${pid}:${startTime}` : undefined;
      }
      visited.add(currentPid);
      const stat = readFileSync(
        `${currentPid === Number(pid) ? processPath : `/proc/${currentPid}`}/stat`,
        "utf8",
      );
      const fields = stat
        .slice(stat.lastIndexOf(")") + 2)
        .trim()
        .split(/\s+/);
      startTime ??= fields[19];
      const parentPid = Number(fields[1]);
      if (!Number.isInteger(parentPid) || parentPid <= 0) return undefined;
      currentPid = parentPid;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

function managedCredentialProcess(
  agentctlPid: number,
  sessionId: string,
): ManagedCredentialProcess | undefined {
  const matches: ManagedCredentialProcess[] = [];
  for (const entry of readdirSync("/proc")) {
    if (!/^\d+$/.test(entry)) continue;
    const processPath = `/proc/${entry}`;
    try {
      const identity = agentProcessIdentityWithinAgentctl(processPath, entry, agentctlPid);
      if (!identity) continue;
      if (readFileSync(`${processPath}/comm`, "utf8").trim() !== "mock-agent") continue;
      const env = new Map(
        readFileSync(`${processPath}/environ`)
          .toString()
          .split("\0")
          .filter(Boolean)
          .map((value): [string, string] => {
            const separator = value.indexOf("=");
            return [value.slice(0, separator), value.slice(separator + 1)];
          }),
      );
      if (env.get("KANDEV_GITHUB_CREDENTIAL_SESSION_ID") !== sessionId) continue;
      const lease = env.get("KANDEV_GITHUB_CREDENTIAL_LEASE") ?? "";
      const reissueCapability = env.get("KANDEV_GITHUB_CREDENTIAL_REISSUE_CAPABILITY") ?? "";
      matches.push({
        identity,
        pid: Number(entry),
        hasLease: lease.length > 0,
        hasReissueCapability: reissueCapability.length > 0,
        reissueCapability,
      });
    } catch {
      // A process can exit between the /proc directory read and file access.
    }
  }
  return matches.length === 1 ? matches[0] : undefined;
}

function latestGitCredentialReissueSignerSource(logPath: string): string | undefined {
  const lines = readFileSync(logPath, "utf8")
    .split("\n")
    .filter((line) => line.includes("git_credential_reissue.signer_selected"));
  const match = lines.at(-1)?.match(/"source"\s*:\s*"([^"]+)"/);
  return match?.[1];
}

// AC-EXECUTORS-SURVIVAL-001..003: with the capability enabled, a worktree
// executor's agent process is never stopped as part of a graceful backend
// restart (kill-paths #4/#5 are closed), so a turn already in flight when the
// backend goes down keeps running on its own and the re-tracked backend finds
// it exactly as it left it — no relaunch, no interrupted marker, and no
// premature move out of IN_PROGRESS on account of the restart itself
// (AC-EXECUTORS-SURVIVAL-003.5). The turn still completes normally once it
// finishes, and the workflow engine's own on_turn_complete transition then
// moves the task to REVIEW exactly as it would for an uninterrupted run
// (see session-resume.spec.ts for that baseline). `backend.restart()` sends
// SIGTERM to the backend's own process group only; agentctl already runs in
// its own group (see backend CLAUDE.md), so it is unaffected regardless of
// the capability — what the capability controls is whether the backend's own
// graceful-shutdown path stops it on the way down.
test.describe("Agent survival across backend restart", () => {
  test.describe.configure({ retries: 0 });

  test("a worktree session's in-flight turn survives a graceful backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const traffic = attachGatewayTrafficCapture(testPage);
    const messageAdds = attachMessageAddCapture(testPage);

    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_AGENT_SURVIVAL: "true",
    });
    const turn = survivalTurnGate(backend.tmpDir);

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Agent Survival Restart Task",
        seedData.agentProfileId,
        {
          description: turn.prompt,
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      // Confirm the agent actually launched, and that its turn is genuinely
      // in flight (past the initial "thinking" step), before pulling the
      // backend out from under it.
      const bootMessage = session.chat.getByText(/Started agent|Resumed agent/i);
      await expect(bootMessage).toBeVisible({ timeout: 30_000 });
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Running slow response", { exact: false }),
      ).toBeVisible({
        timeout: 30_000,
      });
      await expect(apiClient.getTask(task.id)).resolves.toMatchObject({ state: "IN_PROGRESS" });
      const liveSessionID = (await apiClient.listTaskSessions(task.id)).sessions.find(
        (item) => item.is_primary,
      )?.id;
      expect(liveSessionID).toBeTruthy();
      const beforeRestart = await captureSilentRestartSnapshot(apiClient, task.id, liveSessionID!);
      expect(beforeRestart.agentExecutionId).toBeTruthy();
      const messageAddsBeforeRestart = countSessionMessageAdds(
        messageAdds.frames,
        task.id,
        liveSessionID!,
      );

      // Restart the backend while the turn is still running.
      await backend.restart();

      // Reload so the page re-hydrates from the restarted backend's own SSR
      // boot payload, not just a live WS event the still-open tab happened to
      // receive.
      await testPage.reload();
      await session.waitForLoad();

      const reattachedTask = await apiClient.getTask(task.id);
      expect(reattachedTask.state).toBe("IN_PROGRESS");
      const reattachedSessions = await apiClient.listTaskSessions(task.id);
      const reattachedSession = reattachedSessions.sessions.find((item) => item.is_primary);
      expect(reattachedSession?.id).toBe(liveSessionID);
      expect(reattachedSession?.state).toBe("RUNNING");
      await expectRestartConversationIdentity({
        page: testPage,
        api: apiClient,
        taskId: task.id,
        snapshot: beforeRestart,
        traffic: traffic.frames,
        expectedState: "RUNNING",
        executionDisposition: "preserved",
      });
      expect(countSessionMessageAdds(messageAdds.frames, task.id, liveSessionID!)).toBe(
        messageAddsBeforeRestart,
      );

      // The instance was re-tracked, not relaunched: exactly the one boot
      // message from the original launch, no second "Started agent" and no
      // "Resumed agent" from a resume flow this session never needed.
      await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toHaveCount(1);

      // No manual-recovery banner: a re-tracked session is not an interrupted
      // one.
      await expect(session.recoveryFreshButton()).toHaveCount(0);
      await expect(session.recoveryResumeButton()).toHaveCount(0);

      // The turn that was still running across the restart must complete on
      // its own and report normally, rather than being abandoned into an idle
      // waiting-for-input state by startup reconciliation.
      await turn.release();
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Slow response complete", { exact: false }),
      ).toBeVisible({
        timeout: 30_000,
      });
      await session.waitForChatIdle({ timeout: 15_000 });
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Slow response complete", { exact: false }),
      ).toHaveCount(1);

      // The turn genuinely finished, so the workflow engine's own
      // on_turn_complete transition moves the task to REVIEW exactly as it
      // would without any restart (session-resume.spec.ts exercises the same
      // seedData workflow's baseline). What AC-EXECUTORS-SURVIVAL-003.5 rules
      // out is a move to REVIEW *on account of the restart itself* — the task
      // must never gain the interrupted marker.
      await expect
        .poll(async () => (await apiClient.getTask(task.id)).state, {
          timeout: 30_000,
          message: "Task did not advance to REVIEW after the turn completed",
        })
        .toBe("REVIEW");
      const reconciledTask = await apiClient.getTask(task.id);
      expect(reconciledTask.metadata?.interrupted_at).toBeUndefined();

      // The agent still works after the restart: a fresh prompt gets a
      // response through the re-tracked, still-live instance.
      await session.sendMessage("/e2e:simple-message");
      await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
      await session.waitForChatIdle({ timeout: 45_000 });
      expect(countSessionMessageAdds(messageAdds.frames, task.id, liveSessionID!)).toBe(
        messageAddsBeforeRestart + 1,
      );

      // The completed Review task remains in the sidebar even when it has no
      // active board column. Its row must not advertise an interrupted turn.
      const taskRow = session.sidebarTaskItem("Agent Survival Restart Task");
      await expect(taskRow).toBeVisible();
      await expect(taskRow.getByTestId("task-state-interrupted")).toHaveCount(0);
    } finally {
      await turn.release();
      await releaseFeature();
    }
  });

  test("a local_pc session's in-flight turn survives a graceful backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const traffic = attachGatewayTrafficCapture(testPage);
    const messageAdds = attachMessageAddCapture(testPage);

    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_AGENT_SURVIVAL: "true",
    });
    const turn = survivalTurnGate(backend.tmpDir);

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Local PC Agent Survival Restart Task",
        seedData.agentProfileId,
        {
          description: turn.prompt,
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: await localExecutorProfileId(apiClient),
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toBeVisible({
        timeout: 30_000,
      });
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Running slow response", { exact: false }),
      ).toBeVisible({
        timeout: 30_000,
      });
      const liveSessionID = (await apiClient.listTaskSessions(task.id)).sessions.find(
        (item) => item.is_primary,
      )?.id;
      expect(liveSessionID).toBeTruthy();
      const beforeRestart = await captureSilentRestartSnapshot(apiClient, task.id, liveSessionID!);
      expect(beforeRestart.agentExecutionId).toBeTruthy();
      const messageAddsBeforeRestart = countSessionMessageAdds(
        messageAdds.frames,
        task.id,
        liveSessionID!,
      );

      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();

      const reattachedTask = await apiClient.getTask(task.id);
      expect(reattachedTask.state).toBe("IN_PROGRESS");
      const reattachedSessions = await apiClient.listTaskSessions(task.id);
      const reattachedSession = reattachedSessions.sessions.find((item) => item.is_primary);
      expect(reattachedSession?.id).toBe(liveSessionID);
      expect(reattachedSession?.state).toBe("RUNNING");
      await expectRestartConversationIdentity({
        page: testPage,
        api: apiClient,
        taskId: task.id,
        snapshot: beforeRestart,
        traffic: traffic.frames,
        expectedState: "RUNNING",
        executionDisposition: "preserved",
      });
      expect(countSessionMessageAdds(messageAdds.frames, task.id, liveSessionID!)).toBe(
        messageAddsBeforeRestart,
      );

      await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toHaveCount(1);
      await expect(session.recoveryFreshButton()).toHaveCount(0);
      await expect(session.recoveryResumeButton()).toHaveCount(0);
      await turn.release();
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Slow response complete", { exact: false }),
      ).toBeVisible({
        timeout: 30_000,
      });
      await session.waitForChatIdle({ timeout: 15_000 });
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Slow response complete", { exact: false }),
      ).toHaveCount(1);

      await session.sendMessage("/e2e:simple-message");
      await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
      await session.waitForChatIdle({ timeout: 45_000 });
      expect(countSessionMessageAdds(messageAdds.frames, task.id, liveSessionID!)).toBe(
        messageAddsBeforeRestart + 1,
      );
    } finally {
      await turn.release();
      await releaseFeature();
    }
  });

  test("restarts an adopted lease-only session with a fresh managed Git capability", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(150_000);
    test.skip(process.platform !== "linux", "requires Linux /proc process environment inspection");
    const releaseSurvival = await backend.useEnv({ KANDEV_FEATURES_AGENT_SURVIVAL: "true" });
    let releaseReissueDisable = await backend.useEnv({
      KANDEV_GITHUB_CREDENTIAL_BROKER_DISABLE_REISSUE: "true",
    });
    let releaseReissueEnabled: () => Promise<void> = async () => {};
    const turn = survivalTurnGate(backend.tmpDir);
    let repositoryId: string | undefined;
    let sessionId: string | undefined;
    let workspaceCredentialsBaseline: WorkspaceGitCredentialsBaseline | undefined;

    try {
      workspaceCredentialsBaseline = await captureWorkspaceGitCredentialsBaseline(
        apiClient,
        seedData.workspaceId,
      );

      await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
        source: "pat",
        status: "active",
        login: "lease-recovery-e2e",
      });
      const settings = await apiClient.rawRequest("PUT", "/api/v1/github/workspace-settings", {
        workspace_id: seedData.workspaceId,
        task_git_credentials_mode: "managed",
      });
      expect(settings.status).toBe(200);

      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        seedData.repositoryPath,
        "main",
        {
          name: "acme/widgets managed credential recovery",
          provider: "github",
          provider_host: "https://github.com",
          provider_owner: "acme",
          provider_name: "widgets",
        },
      );
      repositoryId = repository.id;
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Managed Git lease recovery after backend restart",
        seedData.agentProfileId,
        {
          description: turn.prompt,
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [repository.id],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );
      sessionId = task.session_id;
      if (!sessionId) throw new Error("managed credential task did not create a session");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toBeVisible({
        timeout: 30_000,
      });
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Running slow response", { exact: false }),
      ).toBeVisible({ timeout: 30_000 });

      const before = await captureSilentRestartSnapshot(apiClient, task.id, sessionId);
      expect(before.agentExecutionId).toBeTruthy();
      const initialAgentctl = managedAgentctlIdentity(backend.tmpDir, sessionId);
      if (!initialAgentctl) throw new Error("managed session has no agentctl execution record");
      expect(initialAgentctl.executionId).toBe(before.agentExecutionId);
      const initialAgentctlProcess = agentctlProcessForPort(initialAgentctl.agentctlPort);
      if (!initialAgentctlProcess)
        throw new Error("managed session agentctl listener process was not found");
      await expect
        .poll(
          () => managedCredentialProcess(initialAgentctlProcess.pid, sessionId!)?.hasLease ?? false,
          {
            timeout: 15_000,
            message: "managed launch did not retain its lease in its agentctl instance",
          },
        )
        .toBe(true);
      const leaseOnlyProcess = managedCredentialProcess(initialAgentctlProcess.pid, sessionId);
      if (!leaseOnlyProcess) throw new Error("unique managed agentctl child process was not found");
      expect(leaseOnlyProcess.hasReissueCapability).toBe(false);

      await releaseReissueDisable();
      releaseReissueDisable = async () => undefined;
      releaseReissueEnabled = await backend.useEnv({
        KANDEV_GITHUB_CREDENTIAL_BROKER_DISABLE_REISSUE: "false",
      });
      expect(latestGitCredentialReissueSignerSource(backend.logPath)).toBe("installation");
      await testPage.reload();
      await session.waitForLoad();
      await expectRestartConversationIdentity({
        page: testPage,
        api: apiClient,
        taskId: task.id,
        snapshot: before,
        traffic: [],
        expectedState: "RUNNING",
        executionDisposition: "preserved",
      });
      const adoptedAgentctl = managedAgentctlIdentity(backend.tmpDir, sessionId);
      expect(adoptedAgentctl).toEqual(initialAgentctl);
      const adoptedAgentctlProcess = agentctlProcessForPort(adoptedAgentctl!.agentctlPort);
      expect(adoptedAgentctlProcess).toEqual(initialAgentctlProcess);
      const adoptedProcess = managedCredentialProcess(adoptedAgentctlProcess!.pid, sessionId);
      expect(adoptedProcess?.identity).toBe(leaseOnlyProcess.identity);
      expect(adoptedProcess?.hasLease).toBe(true);
      expect(adoptedProcess?.hasReissueCapability).toBe(false);

      await turn.release();
      await expect(
        session.chat
          .locator("[data-agent-message-body]")
          .getByText("Slow response complete", { exact: false }),
      ).toBeVisible({ timeout: 30_000 });
      await session.waitForChatIdle({ timeout: 30_000 });

      await session.sessionTabBySessionId(sessionId).click({ button: "right" });
      await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).click();
      await expect
        .poll(
          async () => {
            const executionId = (await apiClient.getTaskSession(sessionId!)).session
              .agent_execution_id;
            return Boolean(executionId) && executionId !== before.agentExecutionId;
          },
          {
            timeout: 30_000,
            message: "Restart session did not create a populated fresh execution",
          },
        )
        .toBe(true);
      const relaunchedSession = (await apiClient.getTaskSession(sessionId)).session;
      expect(relaunchedSession.agent_execution_id).toBeTruthy();
      expect(relaunchedSession.agent_execution_id).not.toBe(before.agentExecutionId);
      expect(relaunchedSession.id).toBe(sessionId);
      expect(relaunchedSession.task_id).toBe(task.id);
      expect(relaunchedSession.task_environment_id).toBe(before.taskEnvironmentId);
      const relaunchedAgentctl = managedAgentctlIdentity(backend.tmpDir, sessionId);
      if (!relaunchedAgentctl)
        throw new Error("restarted session has no agentctl execution record");
      expect(relaunchedAgentctl.executionId).toBe(relaunchedSession.agent_execution_id);

      await expect
        .poll(
          () => {
            const current = managedAgentctlIdentity(backend.tmpDir, sessionId!);
            if (current?.executionId !== relaunchedSession.agent_execution_id) return false;
            const controller = agentctlProcessForPort(current.agentctlPort);
            return controller
              ? (managedCredentialProcess(controller.pid, sessionId!)?.hasReissueCapability ??
                  false)
              : false;
          },
          {
            timeout: 15_000,
            message: "fresh managed agentctl child process was not found",
          },
        )
        .toBe(true);
      const relaunchedAgentctlProcess = agentctlProcessForPort(relaunchedAgentctl.agentctlPort);
      if (!relaunchedAgentctlProcess)
        throw new Error("restarted session agentctl listener process was not found");
      expect(relaunchedAgentctlProcess.identity).toBe(initialAgentctlProcess.identity);
      const freshProcess = managedCredentialProcess(relaunchedAgentctlProcess.pid, sessionId);
      if (!freshProcess)
        throw new Error("unique fresh managed agentctl child process was not found");
      expect(freshProcess.identity).not.toBe(leaseOnlyProcess.identity);
      expect(freshProcess.hasLease).toBe(true);
      expect(freshProcess.hasReissueCapability).toBe(true);

      const reissue = await apiClient.rawRequest("POST", "/api/v1/git/credentials/reissue", {
        reissue_capability: freshProcess.reissueCapability,
        task_id: task.id,
        session_id: sessionId,
        repository_id: repository.id,
        owner: "acme",
        repo: "widgets",
        host: "github.com",
      });
      const reissueStatus = reissue.status;
      await reissue.body?.cancel();
      expect(reissueStatus).toBe(200);
    } finally {
      await turn.release();
      if (sessionId) await apiClient.stopSession({ session_id: sessionId }).catch(() => undefined);
      if (workspaceCredentialsBaseline) {
        await restoreWorkspaceGitCredentialsBaseline(
          apiClient,
          seedData.workspaceId,
          workspaceCredentialsBaseline,
        );
      }
      if (repositoryId) await apiClient.deleteRepository(repositoryId).catch(() => undefined);
      await releaseReissueEnabled();
      await releaseReissueDisable();
      await releaseSurvival();
    }
  });
});

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
test("slow native restore preserves the conversation and accepts a new instruction", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(180_000);
  await verifyAutomaticRestartReconciliation(testPage, apiClient, seedData, backend, "40s");
});
