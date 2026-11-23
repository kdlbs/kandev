import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

export type CopilotContextInfo = {
  contextTier: "default" | "long_context";
  pid: number;
  sessionId: string;
  loads: number;
  priorTurns: number;
};

const COPILOT_CONTEXT_INFO_RE =
  /Copilot runtime context_tier=(default|long_context) pid=(\d+) session=(\S+) loads=(\d+) prior_turns=(\d+)/;

export function parseCopilotContextInfo(content: string): CopilotContextInfo | null {
  const match = COPILOT_CONTEXT_INFO_RE.exec(content);
  if (!match) return null;
  return {
    contextTier: match[1] as "default" | "long_context",
    pid: Number(match[2]),
    sessionId: match[3],
    loads: Number(match[4]),
    priorTurns: Number(match[5]),
  };
}

export async function enableCopilotNativeBackend(backend: BackendContext) {
  return backend.useEnv({
    KANDEV_E2E_MOCK_COPILOT: "true",
    KANDEV_MOCK_COPILOT_NATIVE: "true",
  });
}

export async function createDisposableCopilotProfile(
  apiClient: ApiClient,
  configOptions?: Record<string, string>,
) {
  const { agents } = await apiClient.listAgents();
  const copilot = agents.find((agent) => agent.name === "copilot-acp");
  if (!copilot) {
    throw new Error("Copilot agent is unavailable after enabling KANDEV_E2E_MOCK_COPILOT");
  }
  const suffix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
  const profile = await apiClient.createAgentProfile(copilot.id, `Copilot context ${suffix}`, {
    model: "mock-fast",
    ...(configOptions ? { config_options: configOptions } : {}),
  });
  return {
    agentId: copilot.id,
    agentName: copilot.name,
    profileId: profile.id,
    async dispose() {
      await apiClient.deleteAgentProfile(profile.id, true);
    },
  };
}

export async function startCopilotContextTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  profileId: string,
  prompt = "/e2e:copilot-context-info",
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Copilot context length",
    profileId,
    {
      description: prompt,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Expected the Copilot task to start a session");

  await waitForAgentMessage(apiClient, task.session_id, "Copilot runtime context_tier=", 30_000);
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    timeout: 30_000,
    message: "Initial Copilot turn must settle before live context-window change",
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return { taskId: task.id, sessionId: task.session_id, session };
}

export async function waitForCopilotContextInfoMessages(
  apiClient: ApiClient,
  sessionId: string,
  expectedCount: number,
): Promise<CopilotContextInfo[]> {
  let parsed: CopilotContextInfo[] = [];
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        parsed = messages
          .filter((item) => item.author_type === "agent")
          .map((item) => parseCopilotContextInfo(item.content))
          .filter((item): item is CopilotContextInfo => item !== null);
        return parsed.length;
      },
      {
        timeout: 30_000,
        message: `Waiting for ${expectedCount} Copilot runtime context-info messages`,
      },
    )
    .toBe(expectedCount);
  return parsed;
}
