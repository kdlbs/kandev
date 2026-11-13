import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { SessionPage } from "../pages/session-page";
import { pollUntil } from "./poll-until";
import { readMockACPTrace } from "./transient-turn-runtime-continuity";

type SessionMessage = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];

export type StartCreatedPromptFixture = {
  taskId: string;
  sessionId: string;
  workflowId: string;
  stepId: string;
  profileId: string;
  tracePath: string;
  dispose: () => Promise<void>;
};

export type StartCreatedPromptFixtureOptions = {
  stepPrompt: string;
  planContent?: string;
};

function countOccurrences(value: string, needle: string): number {
  return value.split(needle).length - 1;
}

function readOptionalMockACPTrace(tracePath: string) {
  if (!fs.existsSync(tracePath)) return [];
  return readMockACPTrace(tracePath);
}

export async function createStartCreatedPromptFixture(
  backend: BackendContext,
  apiClient: ApiClient,
  seedData: SeedData,
  options: StartCreatedPromptFixtureOptions,
): Promise<StartCreatedPromptFixture> {
  const stamp = Date.now();
  const tracePath = path.join(backend.tmpDir, `start-created-prompt-${stamp}.jsonl`);
  const workflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    `Prompt integrity ${stamp}`,
  );
  let stepId = "";
  let profileId = "";
  let taskId = "";

  const dispose = async () => {
    try {
      if (taskId) await apiClient.deleteTask(taskId);
    } catch (error) {
      console.warn("start-created fixture task cleanup failed", error);
    }
    try {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true);
    } catch (error) {
      console.warn("start-created fixture profile cleanup failed", error);
    }
    try {
      await apiClient.deleteWorkflow(workflow.id);
    } catch (error) {
      console.warn("start-created fixture workflow cleanup failed", error);
    }
    fs.rmSync(tracePath, { force: true });
  };

  try {
    const step = await apiClient.createWorkflowStep(workflow.id, "Review", 0, {
      is_start_step: true,
    });
    stepId = step.id;
    await apiClient.updateWorkflowStep(stepId, { prompt: options.stepPrompt });

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((candidate) => candidate.name === "mock-agent");
    if (!agent) throw new Error("mock-agent was not registered");
    const profile = await apiClient.createAgentProfile(agent.id, `Prompt integrity ${stamp}`, {
      model: "mock-fast",
      auto_fallback: false,
      env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
    });
    profileId = profile.id;

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Prompt integrity ${stamp}`,
      profileId,
      {
        description: "",
        start_agent: false,
        prepare_session: true,
        workflow_id: workflow.id,
        workflow_step_id: stepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("prepared task has no session ID");

    await pollUntil(
      async () =>
        (await apiClient.listTaskSessions(taskId)).sessions.find((s) => s.id === task.session_id),
      (session) => session?.state === "CREATED",
      30_000,
      "waiting for the prepared session to reach CREATED",
    );

    await apiClient.wsRequest("task.plan.create", {
      task_id: taskId,
      title: "Retained launch plan",
      content: options.planContent ?? "1. Keep the accepted launch brief.\n2. Do not edit files.",
    });

    return {
      taskId,
      sessionId: task.session_id,
      workflowId: workflow.id,
      stepId,
      profileId,
      tracePath,
      dispose,
    };
  } catch (error) {
    await dispose();
    throw error;
  }
}

export function expectNoNativePromptBeforeLaunch(fixture: StartCreatedPromptFixture) {
  const records = readOptionalMockACPTrace(fixture.tracePath);
  expect(records.filter((record) => record.event === "prompt")).toHaveLength(0);
}

export async function launchExplicitBrief(
  apiClient: ApiClient,
  fixture: StartCreatedPromptFixture,
  brief: string,
) {
  const launched = await apiClient.launchSession({
    task_id: fixture.taskId,
    session_id: fixture.sessionId,
    intent: "start_created",
    agent_profile_id: fixture.profileId,
    prompt: brief,
  });
  expect(launched.session_id).toBe(fixture.sessionId);
}

export async function waitForExplicitLaunchMessage(
  apiClient: ApiClient,
  fixture: StartCreatedPromptFixture,
): Promise<SessionMessage> {
  return pollUntil(
    async () => {
      const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
      const userMessages = messages.filter((message) => message.author_type === "user");
      return userMessages.length === 1 ? userMessages[0] : undefined;
    },
    (message): message is SessionMessage => message !== undefined,
    60_000,
    "waiting for exactly one stored user message for the target session",
  );
}

export function assertExplicitLaunchMessage(
  message: SessionMessage,
  brief: string,
  stepPrompt?: string,
): string {
  expect(message.content).toContain(brief);
  expect(countOccurrences(message.content, brief)).toBe(1);
  expect(
    message.raw_content,
    "the stored raw prompt is required for native comparison",
  ).toBeTruthy();
  const raw = message.raw_content ?? "";
  expect(raw).toContain(brief);
  expect(countOccurrences(raw, brief)).toBe(1);
  if (stepPrompt) {
    expect(message.content).toContain(stepPrompt);
  }
  return raw;
}

export function assertExplicitLaunchACPTrace(
  fixture: StartCreatedPromptFixture,
  message: SessionMessage,
  brief: string,
  stepPrompt?: string,
) {
  const records = readMockACPTrace(fixture.tracePath);
  const sessionNew = records.filter((record) => record.event === "session_new");
  expect(sessionNew).toHaveLength(1);
  expect(sessionNew[0].session_id).toBeTruthy();
  expect(sessionNew[0].process_id).toBeTruthy();
  expect(sessionNew[0].connection_id).toBeTruthy();

  const prompts = records.filter((record) => record.event === "prompt");
  expect(prompts).toHaveLength(1);
  const prompt = prompts[0];
  expect(prompt.session_id).toBe(sessionNew[0].session_id);
  expect(prompt.process_id).toBe(sessionNew[0].process_id);
  expect(prompt.connection_id).toBe(sessionNew[0].connection_id);

  const storedRaw = message.raw_content;
  expect(storedRaw, "the stored raw prompt is required for native comparison").toBeTruthy();
  expect(prompt.prompt).toBe(storedRaw);
  expect(countOccurrences(prompt.prompt ?? "", brief)).toBe(1);
  if (stepPrompt) {
    expect(prompt.prompt).toContain(stepPrompt);
  }
}

export async function waitForExplicitLaunchACPTrace(fixture: StartCreatedPromptFixture) {
  await pollUntil(
    () => readOptionalMockACPTrace(fixture.tracePath),
    (records) => records.some((record) => record.event === "prompt"),
    60_000,
    "waiting for the first native ACP prompt",
  );
}

export async function expectExplicitLaunchVisible(session: SessionPage, brief: string) {
  const bubble = session.activeChat().getByTestId("user-message-bubble").filter({ hasText: brief });
  await expect(bubble).toHaveCount(1);
  await expect(bubble).toContainText(brief);
}
