import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import {
  advanceCapacityRetries,
  freezeCapacityRetries,
  waitForCapacityRetry,
} from "./capacity-retry-clock";
import {
  readMockACPTrace,
  waitForRetainedTurnFailure,
  type MockACPTrace,
} from "./transient-turn-runtime-continuity";

export type OpenCodeCapacityScenario = "completed-tools" | "after-tools";

const OPENCODE_AGENT_NAME = "opencode-acp";
const OPENCODE_CONTINUATION_TEXT =
  "OpenCode provider continued the unfinished request without repeating completed work.";

/**
 * Production-shaped OpenCode capacity fixture.
 *
 * The mock binary is registered under the real `opencode-acp` identity through
 * KANDEV_MOCK_PROVIDERS, so the ACP adapter selects the production OpenCode
 * dialect and the routing classifier reads the real admission-refusal text.
 * The mock-only capability never grants eligibility here.
 */
export async function createOpenCodeCapacityFixture(
  backend: BackendContext,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario: OpenCodeCapacityScenario,
  options: { initialPrompt: string },
) {
  const tracePath = path.join(backend.tmpDir, `opencode-capacity-${Date.now()}.jsonl`);
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
      KANDEV_DEBUG_PPROF_ENABLED: "true",
      KANDEV_MOCK_PROVIDERS: OPENCODE_AGENT_NAME,
    });
    await freezeCapacityRetries(backend);
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((candidate) => candidate.name === OPENCODE_AGENT_NAME);
    if (!agent) throw new Error("opencode-acp was not registered");
    const profile = await apiClient.createAgentProfile(
      agent.id,
      `OpenCode capacity ${Date.now()}`,
      {
        model: "mock-fast",
        auto_fallback: false,
        env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
      },
    );
    profileId = profile.id;
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `OpenCode capacity ${scenario}`,
      profile.id,
      {
        description: options.initialPrompt,
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
      console.warn("OpenCode capacity fixture cleanup failed", cleanupError);
    }
    throw error;
  }
}

/**
 * Drives attempts through the frozen backend clock, one notice at a time.
 * A relative provider hint is applied at its exact observation time, so the
 * displayed whole-second value can be up to one second short of the armed
 * deadline. Advance one extra second rather than sleep.
 */
export async function driveOpenCodeCapacityRetries(
  backend: BackendContext,
  apiClient: ApiClient,
  sessionId: string,
  from: number,
  to: number,
) {
  for (let attempt = from; attempt <= to; attempt++) {
    const notice = await waitForCapacityRetry(apiClient, sessionId, attempt);
    await advanceCapacityRetries(backend, Number(notice!.metadata!.retry_in_seconds) + 1);
  }
}

export function readOpenCodeCapacityTrace(tracePath: string): MockACPTrace[] {
  return readMockACPTrace(tracePath);
}

/**
 * One initialized process, connection, and native session; exactly one prompt
 * per original request and host dispatch; no restore request during recovery.
 */
export function assertOpenCodeCapacityTrace(
  tracePath: string,
  expectedPrompts: number,
  options: { completedEffects: number } = { completedEffects: 1 },
) {
  const records = readOpenCodeCapacityTrace(tracePath);
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
  const effects = records.filter((record) => record.event === "completed_side_effect");
  expect(effects).toHaveLength(options.completedEffects);
  if (options.completedEffects > 0) {
    expect(effects[0]).toMatchObject({
      scenario: "opencode-completed-tools",
      effect: "fixture.txt",
    });
  }
}

export async function expectOpenCodeCapacityNotice(session: SessionPage) {
  const card = session.transientRetryCard();
  await expect(card).toBeVisible();
  await expect(card).toContainText("of 11");
  // The capacity notice replaces the generic session-recovery controls.
  await expect(session.activeChat().getByTestId("session-recovery-card")).toHaveCount(0);
  await expect(session.recoveryResumeButton()).toHaveCount(0);
  return card;
}

export async function expectOpenCodeContinuation(session: SessionPage) {
  await expect(
    session.activeChat().getByText(OPENCODE_CONTINUATION_TEXT, { exact: true }),
  ).toBeVisible();
  await expect(session.transientRetryCard()).toHaveCount(0);
}

export async function openSecondViewer(page: Page, taskId: string): Promise<Page> {
  const viewer = await page.context().newPage();
  await viewer.goto(`/t/${taskId}`);
  const viewerSession = new SessionPage(viewer);
  await viewerSession.waitForLoad();
  return viewer;
}

/**
 * Confirms the notice survives reload and a second viewer without another
 * host dispatch.
 */
export async function assertSingleNoticeAcrossViewers(
  page: Page,
  session: SessionPage,
  taskId: string,
  expectedPrompts: () => number,
) {
  const before = expectedPrompts();
  await page.reload();
  await session.waitForLoad();
  await expect(session.transientRetryCard()).toHaveCount(1);
  const viewer = await openSecondViewer(page, taskId);
  try {
    await expect(new SessionPage(viewer).transientRetryCard()).toHaveCount(1);
    expect(expectedPrompts()).toBe(before);
  } finally {
    await viewer.close();
  }
}

export async function expectOpenCodeRetainedRefusal(
  apiClient: ApiClient,
  sessionId: string,
  disposition: "refused" | "exhausted" | "window_expired",
) {
  const failure = await waitForRetainedTurnFailure(apiClient, sessionId, disposition);
  expect(failure.content).toContain("ttft_admission_refused");
  expect(failure.metadata).toMatchObject({
    variant: "error",
    runtime_retained: true,
    recovery_actions: false,
  });
  return failure;
}
