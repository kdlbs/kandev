import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { pollUntil } from "./poll-until";
import { waitForSessionState } from "./session";

const PRIVATE_MISE_SENTINEL = "private-startup-mise-427";
const PRIVATE_GENERIC_SENTINEL = "private-startup-generic-427";

type ProcessFailureCause = {
  operation?: string;
  code?: string;
  reason?: string;
  exit_code?: number;
  detail?: string;
};

type ProcessFailureSession = {
  id: string;
  state?: string;
  metadata?: {
    last_agent_error?: {
      stamp?: string;
      occurred_at?: string;
      causes?: ProcessFailureCause[];
    };
  };
};

export type AgentStartupProcessMode = "success" | "mise" | "generic";

export type AgentStartupProcessFixture = {
  taskId: string;
  sessionId: string;
  agentName: string;
  profileId: string;
  statePath: string;
  setMode(mode: AgentStartupProcessMode): void;
  currentFailure(): Promise<ProcessFailureEvidence>;
  waitForNewFailure(
    previousStamp: string | undefined,
    reason: string,
  ): Promise<ProcessFailureEvidence>;
  dispose(): Promise<void>;
};

export type ProcessFailureEvidence = {
  stamp: string;
  reason: string;
  exitCode: number;
  occurredAt: string;
};

/**
 * Run the real agent process and initialize pipeline with a private fake CLI.
 * The CLI delegates to the E2E mock agent until the state file selects a
 * startup refusal, so no real mise installation or provider is involved.
 */
export async function createAgentStartupProcessFixture(
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
  title: string,
  options: { mode?: AgentStartupProcessMode } = {},
): Promise<AgentStartupProcessFixture> {
  const fixtureDir = path.join(backend.tmpDir, "agent-startup-process-diagnostics");
  const statePath = path.join(fixtureDir, "mode.txt");
  const scriptPath = path.join(fixtureDir, "fake-agent.sh");
  prepareFakeAgent(scriptPath, statePath, options.mode ?? "mise");

  let agentName = "";
  let profileId = "";
  let disposed = false;
  try {
    const agent = await apiClient.createCustomTUIAgent({
      display_name: `${title} agent`,
      command: scriptPath,
      protocol: "acp",
    });
    agentName = agent.name;
    const profile = await apiClient.createAgentProfile(agent.id, `${title} profile`, {
      model: "mock-fast",
      cli_passthrough: false,
      env_vars: [{ key: "AGENT_STARTUP_MODE_FILE", value: statePath }],
    });
    profileId = profile.id;
    const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    if (!task.session_id) throw new Error("agent startup task has no session ID");
    const sessionId = task.session_id;
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "FAILED",
      message: "Wait for the private agent startup refusal",
      timeout: 60_000,
    });

    const fixture: AgentStartupProcessFixture = {
      taskId: task.id,
      sessionId,
      agentName,
      profileId,
      statePath,
      setMode(mode) {
        fs.writeFileSync(statePath, mode);
      },
      async currentFailure() {
        return requireFailureEvidence(await readFailureSession(apiClient, task.id, sessionId));
      },
      async waitForNewFailure(previousStamp, reason) {
        let evidence: ProcessFailureEvidence | null = null;
        await pollUntil(
          async () => {
            const session = await readFailureSession(apiClient, task.id, sessionId);
            const candidate = session?.metadata?.last_agent_error;
            if (!candidate?.stamp || candidate.stamp === previousStamp) return null;
            evidence = requireFailureEvidence(session);
            return evidence;
          },
          (value) => value !== null && value.reason === reason,
          60_000,
          `Wait for a new ${reason} process failure`,
        );
        if (!evidence) throw new Error("new process failure evidence was not observed");
        return evidence;
      },
      async dispose() {
        if (disposed) return;
        disposed = true;
        if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
        if (agentName) await apiClient.deleteCustomAgentByName(agentName).catch(() => undefined);
        fs.rmSync(fixtureDir, { recursive: true, force: true });
      },
    };
    assertDurablePrivacy(await durableSessionData(apiClient, task.id, sessionId));
    return fixture;
  } catch (error) {
    if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
    if (agentName) await apiClient.deleteCustomAgentByName(agentName).catch(() => undefined);
    fs.rmSync(fixtureDir, { recursive: true, force: true });
    throw error;
  }
}

function prepareFakeAgent(scriptPath: string, statePath: string, mode: AgentStartupProcessMode) {
  fs.mkdirSync(path.dirname(scriptPath), { recursive: true });
  fs.writeFileSync(statePath, mode);
  fs.writeFileSync(
    scriptPath,
    `#!/bin/sh
set -eu
mode=$(cat "\${AGENT_STARTUP_MODE_FILE:-/dev/null}" 2>/dev/null || printf 'success')
case "$mode" in
  mise)
    printf '%s\\n' "mise ERROR Config files in $PWD/${PRIVATE_MISE_SENTINEL}/mise.toml are not trusted." >&2
    exit 1
    ;;
  generic)
    printf '%s\\n' "${PRIVATE_GENERIC_SENTINEL} $PWD" >&2
    exit 3
    ;;
  *)
    exec mock-agent "$@"
    ;;
esac
`,
    { mode: 0o755 },
  );
  fs.chmodSync(scriptPath, 0o755);
}

async function readFailureSession(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<ProcessFailureSession | undefined> {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions.find((session) => session.id === sessionId) as unknown as
    | ProcessFailureSession
    | undefined;
}

function requireFailureEvidence(
  session: ProcessFailureSession | undefined,
): ProcessFailureEvidence {
  const error = session?.metadata?.last_agent_error;
  const cause = error?.causes?.find((candidate) => candidate.code === "process_exited");
  if (!error?.stamp || !error.occurred_at || !cause || typeof cause.exit_code !== "number") {
    throw new Error(`process failure evidence was incomplete: ${JSON.stringify(error)}`);
  }
  return {
    stamp: error.stamp,
    reason: cause.reason ?? "unclassified",
    exitCode: cause.exit_code,
    occurredAt: error.occurred_at,
  };
}

async function durableSessionData(apiClient: ApiClient, taskId: string, sessionId: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const session = sessions.find((candidate) => candidate.id === sessionId);
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return JSON.stringify({ session, messages });
}

function assertDurablePrivacy(data: string) {
  for (const sentinel of agentStartupProcessPrivacySentinels) {
    if (data.includes(sentinel)) {
      throw new Error(`Raw startup diagnostic escaped into durable session data: ${sentinel}`);
    }
  }
}

export const agentStartupProcessPrivacySentinels = [
  PRIVATE_MISE_SENTINEL,
  PRIVATE_GENERIC_SENTINEL,
];
