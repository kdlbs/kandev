import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { pollUntil } from "./poll-until";

type SessionMessage = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];

export type MockACPTrace = {
  event: string;
  session_id: string;
  process_id: string;
  connection_id: string;
  prompt?: string;
  value?: string;
  scenario?: string;
  effect?: string;
};

export async function createRetainedCapacityFixture(
  backend: BackendContext,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario: string,
) {
  const tracePath = path.join(backend.tmpDir, `retained-capacity-${Date.now()}.jsonl`);
  let profileId = "";
  let taskId = "";
  const dispose = async () => {
    try {
      if (taskId) await apiClient.deleteTask(taskId);
      if (profileId) await apiClient.deleteAgentProfile(profileId, true);
    } finally {
      fs.rmSync(tracePath, { force: true });
      await backend.restart();
    }
  };
  try {
    await backend.restart({ KANDEV_DEBUG_PPROF_ENABLED: "true" });
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((candidate) => candidate.name === "mock-agent");
    if (!agent) throw new Error("mock-agent was not registered");
    const profile = await apiClient.createAgentProfile(agent.id, `Capacity ${Date.now()}`, {
      model: "mock-fast",
      auto_fallback: false,
      env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
    });
    profileId = profile.id;
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Retained capacity ${scenario}`,
      profile.id,
      {
        description: `/capacity-${scenario}`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("created task has no session ID");
    return { taskId, sessionId: task.session_id, tracePath, dispose };
  } catch (error) {
    try {
      await dispose();
    } catch (cleanupError) {
      console.warn("Retained capacity fixture cleanup failed", cleanupError);
    }
    throw error;
  }
}

export async function waitForRetainedTurnFailure(
  apiClient: ApiClient,
  sessionId: string,
  disposition?: string,
): Promise<SessionMessage> {
  return pollUntil(
    async () => {
      const { messages } = await apiClient.listSessionMessages(sessionId);
      return messages.find(
        (message) =>
          message.metadata?.runtime_retained === true &&
          (disposition === undefined || message.metadata.recovery_disposition === disposition),
      );
    },
    (message): message is SessionMessage => message !== undefined,
    180_000,
    `waiting for retained provider failure${disposition ? ` (${disposition})` : ""}`,
  );
}

export function readMockACPTrace(tracePath: string): MockACPTrace[] {
  const content = fs.readFileSync(tracePath, "utf8").trim();
  return content ? content.split("\n").map((line) => JSON.parse(line) as MockACPTrace) : [];
}

export function assertRetainedACPTrace(tracePath: string, expectedPrompts: number) {
  const records = readMockACPTrace(tracePath);
  const nativeRecords = records.filter((record) => record.session_id);
  expect(nativeRecords.length).toBeGreaterThan(0);
  for (const record of nativeRecords) {
    expect(record.process_id).toBeTruthy();
    expect(record.connection_id).toBeTruthy();
    expect(record.session_id).toBeTruthy();
  }
  expect(new Set(nativeRecords.map((record) => record.process_id)).size).toBe(1);
  expect(new Set(nativeRecords.map((record) => record.connection_id)).size).toBe(1);
  expect(new Set(nativeRecords.map((record) => record.session_id)).size).toBe(1);
  expect(records.filter((record) => record.event === "initialize")).toHaveLength(1);
  expect(records.filter((record) => record.event === "session_new")).toHaveLength(1);
  expect(records.filter((record) => record.event === "session_load")).toHaveLength(0);
  expect(records.filter((record) => record.event === "resume")).toHaveLength(0);
  expect(records.filter((record) => record.event === "prompt")).toHaveLength(expectedPrompts);
}

export function assertCompletedCapacityACPTrace(tracePath: string, originalCommand: string) {
  const records = readMockACPTrace(tracePath);
  const prompts = records.filter((record) => record.event === "prompt");
  assertRetainedACPTrace(tracePath, 2);
  expect(prompts.filter((record) => record.prompt?.includes(originalCommand))).toHaveLength(1);
  const effects = records.filter((record) => record.event === "completed_side_effect");
  expect(effects).toHaveLength(1);
  expect(effects[0]).toMatchObject({ scenario: "completed-tools", effect: "fixture.txt" });
}

export function assertRetainedFailureMessage(message: SessionMessage) {
  expect(message.content).toContain("Selected model is at capacity.");
  expect(message.metadata).toMatchObject({
    variant: "error",
    runtime_retained: true,
    recovery_actions: false,
  });
}
