import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { pollUntil } from "./poll-until";

type Message = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];
type Trace = { event: string; session_id: string; prompt?: string };

export async function createContinuationFixture(
  backend: BackendContext,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario: string,
  options: { enabled?: boolean; env?: Record<string, string>; executorProfileId?: string } = {},
) {
  const tracePath = path.join(backend.tmpDir, `continuation-${Date.now()}.jsonl`);
  await backend.restart({
    ...options.env,
    KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION: String(options.enabled ?? true),
    KANDEV_DEBUG_PPROF_ENABLED: "true",
  });
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.name === "mock-agent");
  if (!agent) throw new Error("mock-agent was not registered");
  const profile = await apiClient.createAgentProfile(agent.id, `Continuation ${Date.now()}`, {
    model: "mock-fast",
    auto_fallback: false,
    env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
  });
  let taskId = "";
  const dispose = async () => {
    try {
      if (taskId) await apiClient.deleteTask(taskId);
      await apiClient.deleteAgentProfile(profile.id, true);
    } finally {
      fs.rmSync(tracePath, { force: true });
      await backend.restart();
    }
  };
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Continuation ${scenario}`,
      profile.id,
      {
        description: `/continuation-${scenario}`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: options.executorProfileId,
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("created task has no session ID");
    return { taskId, sessionId: task.session_id, tracePath, dispose };
  } catch (error) {
    await dispose();
    throw error;
  }
}

export async function waitForContinuationMessage(
  api: ApiClient,
  sessionId: string,
  predicate: (message: Message) => boolean,
) {
  return pollUntil(
    async () => (await api.listSessionMessages(sessionId)).messages.find(predicate),
    (message): message is Message => message !== undefined,
    75_000,
    "waiting for persisted continuation evidence",
  );
}

export function assertNativeContinuationTrace(tracePath: string, scenario: string, loads = 1) {
  const records: Trace[] = fs
    .readFileSync(tracePath, "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line) as Trace);
  const originals = records.filter(
    (record) => record.event === "prompt" && record.prompt?.includes(`/continuation-${scenario}`),
  );
  const continuations = records.filter(
    (record) =>
      record.event === "prompt" &&
      record.prompt?.startsWith(
        "Your previous turn was interrupted by a temporary connection failure.",
      ),
  );
  expect(originals).toHaveLength(1);
  expect(continuations).toHaveLength(1);
  const nativeId = originals[0].session_id;
  expect(continuations[0].session_id).toBe(nativeId);
  expect(
    records.filter((record) => record.event === "session_load" && record.session_id === nativeId),
  ).toHaveLength(loads);
  expect(records.filter((record) => record.event === "session_new")).toHaveLength(1);
}
