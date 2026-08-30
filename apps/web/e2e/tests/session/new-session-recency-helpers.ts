import { expect, type Page } from "@playwright/test";
import type { AgentProfile } from "../../../lib/types/http-agents";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { openTaskSession, waitForSessionDone } from "../../helpers/session";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

export async function createRecentUseProfiles(apiClient: ApiClient): Promise<{
  profileA: AgentProfile;
  profileB: AgentProfile;
}> {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.id !== "dynamic") ?? agents[0];
  if (!agent) throw new Error("No launchable agent available in test fixtures");

  const suffix = Date.now().toString(36);
  const profileA = await apiClient.createAgentProfile(agent.id, `Session Recency A ${suffix}`, {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agent.id, `Session Recency B ${suffix}`, {
    model: "mock-fast",
  });
  return { profileA, profileB };
}

export async function seedTaskSessionRecency(options: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  profileA: AgentProfile;
  profileB: AgentProfile;
  mobile?: boolean;
}): Promise<{ targetTaskId: string }> {
  const { testPage, apiClient, seedData, profileA, profileB, mobile = false } = options;
  const sourceTask = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Session Recency Source",
    profileA.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!sourceTask.session_id) throw new Error("Source task did not return a session ID");

  await waitForSessionDone(
    apiClient,
    sourceTask.id,
    sourceTask.session_id,
    "Waiting for the source session to finish",
  );
  const sourceSession = await openTaskSession(testPage, sourceTask.id);
  await sourceSession.waitForChatIdle({ timeout: 30_000 });
  if (mobile) {
    await sourceSession.openMobileNewSessionDialog();
  } else {
    await sourceSession.openNewSessionDialog();
  }
  await expect(sourceSession.newSessionDialog()).toBeVisible({ timeout: 5_000 });
  await sourceSession.newSessionDialogPage.selectProfile(profileB.name, mobile);
  await sourceSession.newSessionPromptInput().fill("/e2e:simple-message");
  if (mobile) {
    await sourceSession.newSessionStartButton().tap();
  } else {
    await sourceSession.newSessionStartButton().click();
  }
  await expect(sourceSession.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(sourceTask.id);
        return sessions.length === 2 && DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 120_000, message: "Waiting for the recency source session" },
    )
    .toBe(true);
  await expect
    .poll(
      async () => {
        const records = await apiClient.listAgentProfileRecentUse();
        return records.find((record) => record.context === "task_session")?.profile_ids[0] ?? "";
      },
      { timeout: 30_000, message: "Waiting for task-session recency to record profile B" },
    )
    .toBe(profileB.id);

  const targetTask = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Session Recency Target",
    profileA.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!targetTask.session_id) throw new Error("Target task did not return a session ID");
  await waitForSessionDone(
    apiClient,
    targetTask.id,
    targetTask.session_id,
    "Waiting for the target session to finish",
  );
  return { targetTaskId: targetTask.id };
}
