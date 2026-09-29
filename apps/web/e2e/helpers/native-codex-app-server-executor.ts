import { randomUUID } from "node:crypto";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { AgentProfile } from "../../lib/types/agent-profile";
import { waitForLatestSessionDone } from "./session";

const nativeCodexFlag = "KANDEV_FEATURES_CODEX_APP_SERVER";

export type NativeCodexExecutorProbe = {
  profile: AgentProfile;
  commandPrefix: string;
  prepareScript: string;
  markerPath: string;
};

export type NativeCodexExecutorSeed = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  executorProfileId: string;
  executorId?: string;
  repositoryId?: string;
};

export async function createNativeCodexExecutorProbe(
  backend: BackendContext,
  apiClient: ApiClient,
  profileName: string,
): Promise<NativeCodexExecutorProbe> {
  await backend.restart({ [nativeCodexFlag]: "true", KANDEV_MOCK_AGENT: "true" });
  await assertNativeCodexRuntimeFlag(backend, true);
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.name === "codex-app-server");
  if (!agent) throw new Error("Codex app-server was not registered while its feature flag was on");

  const id = randomUUID();
  const dir = `/tmp/kandev-native-codex-${id}`;
  const profile = await apiClient.createAgentProfile(agent.id, profileName, {
    model: "gpt-5-codex",
    command_prefix: `${dir}/launch-app-server`,
    env_vars: [{ key: "OPENAI_API_KEY", value: "e2e-placeholder" }],
  });

  return {
    profile,
    commandPrefix: `${dir}/launch-app-server`,
    markerPath: `${dir}/invocation.txt`,
    prepareScript: nativeCodexProbePrepareScript(dir),
  };
}

export async function disableNativeCodexAppServer(
  backend: BackendContext,
  apiClient: ApiClient,
  profileId: string,
): Promise<void> {
  await backend.restart({ [nativeCodexFlag]: "false", KANDEV_MOCK_AGENT: "true" });
  await assertNativeCodexRuntimeFlag(backend, false);
  const preserved = await apiClient.getAgentProfile(profileId);
  if (preserved.id !== profileId) {
    throw new Error("Codex app-server profile was not preserved when the feature flag was off");
  }
}

export async function removeNativeCodexExecutorProbe(
  backend: BackendContext,
  apiClient: ApiClient,
  profileId: string,
): Promise<void> {
  await backend.restart({ [nativeCodexFlag]: "true", KANDEV_MOCK_AGENT: "true" });
  await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
  await backend.restart();
}

export async function expectNativeCodexLaunchBlocked(
  apiClient: ApiClient,
  seed: NativeCodexExecutorSeed,
  profileId: string,
): Promise<string> {
  const task = await apiClient.createTask(seed.workspaceId, "Codex flag-off launch gate", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  try {
    await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: profileId,
      ...(seed.executorId ? { executor_id: seed.executorId } : {}),
      executor_profile_id: seed.executorProfileId,
      workflow_step_id: seed.startStepId,
      prompt: "This launch must be blocked while the native Codex flag is off.",
    });
  } catch (error) {
    return error instanceof Error ? error.message : String(error);
  } finally {
    await apiClient.deleteTask(task.id).catch(() => undefined);
  }
  throw new Error("Codex app-server session launched while its feature flag was off");
}

export async function launchNativeCodexProbe(
  apiClient: ApiClient,
  seed: NativeCodexExecutorSeed,
  profileId: string,
  title: string,
): Promise<string> {
  const task = await apiClient.createTaskWithAgent(seed.workspaceId, title, profileId, {
    description: "Return one short confirmation.",
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    ...(seed.repositoryId ? { repository_ids: [seed.repositoryId] } : {}),
    ...(seed.executorId ? { executor_id: seed.executorId } : {}),
    executor_profile_id: seed.executorProfileId,
  });
  await waitForLatestSessionDone(
    apiClient,
    task.id,
    1,
    "Wait for the native Codex probe session",
    30_000,
  );
  return task.id;
}

export async function assertNativeCodexRuntimeFlag(
  backend: BackendContext,
  expected: boolean,
): Promise<void> {
  const response = await fetch(`${backend.baseUrl}/api/v1/runtime-flags`);
  if (!response.ok) throw new Error(`Runtime flag request failed with status ${response.status}`);
  const payload = (await response.json()) as {
    flags: Array<{ key: string; effective_value: boolean; source: string }>;
  };
  const flag = payload.flags.find((candidate) => candidate.key === "features.codexAppServer");
  if (!flag || flag.effective_value !== expected) {
    throw new Error(
      `Codex app-server runtime flag was ${JSON.stringify(flag)}, expected ${expected}`,
    );
  }
  const agentResponse = await fetch(`${backend.baseUrl}/api/v1/agents`);
  if (!agentResponse.ok) {
    throw new Error(`Agent list request failed with status ${agentResponse.status}`);
  }
  const agentPayload = (await agentResponse.json()) as {
    agents: Array<{ name: string; inference_capable: boolean }>;
  };
  const agent = agentPayload.agents.find((candidate) => candidate.name === "codex-app-server");
  if (!agent || agent.inference_capable !== expected) {
    throw new Error(
      `Codex app-server registration was ${JSON.stringify(agent)}, expected enabled=${expected}`,
    );
  }
}

function nativeCodexProbePrepareScript(dir: string): string {
  return `set -eu
mkdir -p '${dir}'
cat > '${dir}/app-server.mjs' <<'KANDEV_NATIVE_CODEX_SERVER'
import fs from 'node:fs';
import readline from 'node:readline';

fs.writeFileSync('${dir}/invocation.txt', process.argv.slice(2).join(' '));
const input = readline.createInterface({ input: process.stdin });
let turnCount = 0;
function send(frame) { process.stdout.write(JSON.stringify(frame) + '\\n'); }
function notify(method, params) { send({ jsonrpc: '2.0', method, params }); }
input.on('line', (line) => {
  let request;
  try { request = JSON.parse(line); } catch { return; }
  if (request.id === undefined || typeof request.method !== 'string') return;
  const params = request.params || {};
  let result = {};
  if (request.method === 'initialize') {
    result = { userAgent: 'codex-cli 0.154.0' };
  } else if (request.method === 'model/list') {
    result = { data: [{ model: 'gpt-5-codex', displayName: 'GPT-5 Codex', isDefault: true }] };
  } else if (request.method === 'thread/start') {
    result = { thread: { id: 'native-codex-e2e-thread' }, model: 'gpt-5-codex' };
  } else if (request.method === 'turn/start') {
    turnCount += 1;
    const threadId = params.threadId || 'native-codex-e2e-thread';
    const turnId = 'native-codex-e2e-turn-' + turnCount;
    notify('turn/started', { threadId, turn: { id: turnId, status: 'inProgress' } });
    notify('item/agentMessage/delta', { threadId, turnId, itemId: 'native-codex-e2e-message-' + turnCount, delta: 'Native command launch passed.' });
    notify('turn/completed', { threadId, turn: { id: turnId, status: 'completed' } });
    result = { turn: { id: turnId, status: 'completed' } };
  } else if (request.method === 'thread/backgroundTerminals/list') {
    result = { data: [] };
  } else {
    send({ jsonrpc: '2.0', id: request.id, error: { code: -32601, message: 'Method not found' } });
    return;
  }
  send({ jsonrpc: '2.0', id: request.id, result });
});
KANDEV_NATIVE_CODEX_SERVER
cat > '${dir}/launch-app-server' <<'KANDEV_NATIVE_CODEX_LAUNCHER'
#!/bin/sh
set -eu
exec node '${dir}/app-server.mjs' "$@"
KANDEV_NATIVE_CODEX_LAUNCHER
chmod 700 '${dir}/launch-app-server'
`;
}
