import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { pollUntil } from "./poll-until";

type Message = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];
type Trace = {
  event: string;
  session_id: string;
  process_id?: string;
  connection_id?: string;
  prompt?: string;
};

export async function createContinuationFixture(
  backend: BackendContext,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario: string,
  options: { env?: Record<string, string>; executorProfileId?: string } = {},
) {
  const tracePath = path.join(backend.tmpDir, `continuation-${Date.now()}.jsonl`);
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
    await backend.restart({
      ...options.env,
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
    profileId = profile.id;
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
    try {
      await dispose();
    } catch (cleanupError) {
      console.warn("Continuation fixture cleanup failed", cleanupError);
    }
    throw error;
  }
}

export async function assertContinuationSettingRetired(page: Page, backend: BackendContext) {
  const [registryResponse, featuresResponse] = await Promise.all([
    page.request.get(`${backend.baseUrl}/api/v1/runtime-flags`),
    page.request.get(`${backend.baseUrl}/api/v1/features`),
  ]);
  expect(registryResponse.ok()).toBe(true);
  expect(featuresResponse.ok()).toBe(true);

  const registry = (await registryResponse.json()) as { flags: Array<{ key: string }> };
  const features = (await featuresResponse.json()) as Record<string, unknown>;
  expect(registry.flags.map((flag) => flag.key)).not.toContain(
    "features.providerInterruptionContinuation",
  );
  expect(features).not.toHaveProperty("providerInterruptionContinuation");

  await page.goto("/settings/system/feature-toggles");
  const settings = page.getByTestId("feature-toggles-settings");
  await expect(settings).toBeVisible();
  await expect(
    settings.getByText("Interrupted conversation continuation", { exact: true }),
  ).toHaveCount(0);
  const lastFeatureCard = settings.getByTestId("feature-toggle-features.office");
  await expect(lastFeatureCard).toBeVisible();
  await lastFeatureCard.scrollIntoViewIfNeeded();
  await expect(lastFeatureCard).toBeInViewport();
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

export function assertNativeNoContinuationTrace(tracePath: string, scenario: string) {
  const records: Trace[] = fs
    .readFileSync(tracePath, "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line) as Trace);
  const originals = records.filter(
    (record) => record.event === "prompt" && record.prompt?.includes(`/continuation-${scenario}`),
  );
  expect(originals).toHaveLength(1);
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
  expect(records.filter((record) => record.event === "prompt")).toHaveLength(1);
}

export function assertNativeContinuationTrace(tracePath: string, scenario: string, loads = 0) {
  const records: Trace[] = fs
    .readFileSync(tracePath, "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line) as Trace);
  const originals = records.filter(
    (record) => record.event === "prompt" && record.prompt?.includes(`/continuation-${scenario}`),
  );
  const continuations = records.filter(
    (record) => record.event === "prompt" && record.prompt === "continue",
  );
  expect(originals).toHaveLength(1);
  expect(continuations).toHaveLength(1);
  const nativeId = originals[0].session_id;
  expect(nativeId).toBeTruthy();
  expect(continuations[0].session_id).toBe(nativeId);
  const nativeRecords = records.filter((record) => record.session_id === nativeId);
  expect(nativeRecords.length).toBeGreaterThan(0);
  for (const record of nativeRecords) {
    expect(record.process_id).toBeTruthy();
    expect(record.connection_id).toBeTruthy();
    expect(record.session_id).toBeTruthy();
  }
  expect(new Set(nativeRecords.map((record) => record.process_id)).size).toBe(1);
  expect(new Set(nativeRecords.map((record) => record.connection_id)).size).toBe(1);
  expect(records.filter((record) => record.event === "initialize")).toHaveLength(1);
  expect(
    records.filter((record) => record.event === "session_load" && record.session_id === nativeId),
  ).toHaveLength(loads);
  expect(
    records.filter((record) => record.event === "resume" && record.session_id === nativeId),
  ).toHaveLength(loads);
  expect(records.filter((record) => record.event === "session_new")).toHaveLength(1);
}
