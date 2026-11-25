import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

type ACPTrace = {
  event: string;
  session_id: string;
  process_id?: string;
  connection_id?: string;
  prompt?: string;
};

async function runCleanupSteps(
  message: string,
  steps: Array<() => void | Promise<void>>,
): Promise<void> {
  const errors: unknown[] = [];
  for (const step of steps) {
    try {
      await step();
    } catch (error) {
      errors.push(error);
    }
  }
  if (errors.length === 1) throw errors[0];
  if (errors.length > 1) {
    const details = errors.map((error) => (error instanceof Error ? error.message : String(error)));
    throw new Error(`${message}: ${details.join("; ")}`);
  }
}

export function cleanupCopilotInterruptionTest(
  disposeFixture: (() => Promise<void>) | undefined,
  releaseEnvironment: () => Promise<void>,
): Promise<void> {
  return runCleanupSteps("Copilot interruption test cleanup failed", [
    () => disposeFixture?.(),
    releaseEnvironment,
  ]);
}

export async function seedCopilotInterruptionTask(
  page: Page,
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
): Promise<{
  taskId: string;
  sessionId: string;
  tracePath: string;
  dispose: () => Promise<void>;
}> {
  const tracePath = path.join(backend.tmpDir, `copilot-interruption-${Date.now()}.jsonl`);
  let profileId = "";
  let taskId = "";
  const { agents } = await apiClient.listAgents();
  const copilot = agents.find((agent) => agent.name === "copilot-acp");
  if (!copilot) throw new Error("Copilot is unavailable after enabling KANDEV_E2E_MOCK_COPILOT");
  const profile = await apiClient.createAgentProfile(
    copilot.id,
    `Copilot interruption ${Date.now().toString(36)}`,
    {
      model: "mock-fast",
      auto_fallback: false,
      env_vars: [
        { key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath },
        { key: "E2E_MOCK_COPILOT_INTERRUPTION", value: "true" },
      ],
    },
  );
  profileId = profile.id;
  const dispose = async () =>
    runCleanupSteps("Copilot interruption fixture cleanup failed", [
      () => (taskId ? apiClient.deleteTask(taskId) : undefined),
      () => (profileId ? apiClient.deleteAgentProfile(profileId, true) : undefined),
      () => fs.rmSync(tracePath, { force: true }),
    ]);

  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Copilot interrupted tool recovery",
      profile.id,
      {
        description: "/e2e:copilot-interruption-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("Copilot task has no session ID");

    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 45_000 });
    await expect(session.clarificationOverlay()).toContainText(
      "Which database should we use for this project?",
    );
    return { taskId: task.id, sessionId: task.session_id, tracePath, dispose };
  } catch (error) {
    await dispose();
    throw error;
  }
}

export function readCopilotInterruptionTrace(tracePath: string): ACPTrace[] {
  if (!fs.existsSync(tracePath)) return [];
  return fs
    .readFileSync(tracePath, "utf8")
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as ACPTrace);
}

export async function waitForCopilotInterruptionACPStartup(tracePath: string): Promise<ACPTrace> {
  await expect
    .poll(
      () => {
        const records = readCopilotInterruptionTrace(tracePath);
        const setupPrompt = records.find(
          (record) =>
            record.event === "prompt" && record.prompt?.includes("/e2e:copilot-interruption-setup"),
        );
        return records.find(
          (record) =>
            record.event === "session_new" && record.session_id === setupPrompt?.session_id,
        );
      },
      { timeout: 15_000, message: "Waiting for Copilot to start the setup prompt" },
    )
    .toBeTruthy();

  const records = readCopilotInterruptionTrace(tracePath);
  const setupPrompt = records.find(
    (record) =>
      record.event === "prompt" && record.prompt?.includes("/e2e:copilot-interruption-setup"),
  );
  const session = records.find(
    (record) => record.event === "session_new" && record.session_id === setupPrompt?.session_id,
  );
  if (!session?.process_id) throw new Error("Copilot did not record its initial ACP process");
  return session;
}

export async function expectFreshCopilotSessionLoad(
  tracePath: string,
  sessionId: string,
  originalProcessID: string,
): Promise<void> {
  await expect
    .poll(
      () =>
        readCopilotInterruptionTrace(tracePath).some(
          (record) =>
            record.event === "session_load" &&
            record.session_id === sessionId &&
            record.process_id !== originalProcessID,
        ),
      { timeout: 45_000, message: "Waiting for a fresh Copilot process to load the saved session" },
    )
    .toBe(true);
}

export async function expectCopilotInterruptionWarningFrame(
  tracePath: string,
  sessionId: string,
): Promise<void> {
  await expect
    .poll(
      () =>
        readCopilotInterruptionTrace(tracePath).some(
          (record) =>
            record.event === "copilot_interruption_warning" && record.session_id === sessionId,
        ),
      { timeout: 15_000, message: "Waiting for the mock Copilot recovery warning frame" },
    )
    .toBe(true);
}

export async function expectCopilotNoticeBeforeReply(
  apiClient: ApiClient,
  sessionId: string,
): Promise<{ statusID: string; replyID: string }> {
  await expect
    .poll(async () => {
      const { messages } = await apiClient.listSessionMessages(sessionId);
      const statusIndex = messages.findIndex(
        (message) => message.metadata?.copilot_interruption_notice === "clarification",
      );
      const replyIndex = messages.findIndex(
        (message) => message.content === "Here is the next step.",
      );
      return statusIndex >= 0 && replyIndex > statusIndex;
    })
    .toBe(true);

  const { messages } = await apiClient.listSessionMessages(sessionId);
  const statusIndex = messages.findIndex(
    (message) => message.metadata?.copilot_interruption_notice === "clarification",
  );
  const replyIndex = messages.findIndex((message) => message.content === "Here is the next step.");
  const status = messages[statusIndex];
  const reply = messages[replyIndex];
  expect(status.type).toBe("status");
  expect(status.content).toContain("Warning: 1 tool call(s) had no recorded result at resume.");
  expect(status.id).not.toBe(reply.id);
  expect(status.created_at).toBeTruthy();
  expect(reply.created_at).toBeTruthy();
  return { statusID: status.id, replyID: reply.id };
}
