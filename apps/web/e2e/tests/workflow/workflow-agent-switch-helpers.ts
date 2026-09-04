import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export type WorkflowTaskSessions = Awaited<ReturnType<ApiClient["listTaskSessions"]>>["sessions"];

export async function createWorkflowAgentProfiles(apiClient: ApiClient) {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents available in test fixtures");
  const agentId = agents[0].id;
  const profileA = await apiClient.createAgentProfile(agentId, "Profile A (fast)", {
    model: "mock-fast",
  });
  const profileB = await apiClient.createAgentProfile(agentId, "Profile B (slow)", {
    model: "mock-slow",
  });
  return { agentId, profileA, profileB };
}

export async function pollWorkflowTaskSessions(
  apiClient: ApiClient,
  taskId: string,
  expectedCount: number,
  timeoutMs = 30_000,
) {
  let latest: WorkflowTaskSessions = [];
  await expect
    .poll(
      async () => {
        latest = (await apiClient.listTaskSessions(taskId)).sessions;
        return latest.length;
      },
      { timeout: timeoutMs, message: `task ${taskId} never reached ${expectedCount} session(s)` },
    )
    .toBeGreaterThanOrEqual(expectedCount);
  return latest;
}

export async function waitForWorkflowProfileSession(
  apiClient: ApiClient,
  taskId: string,
  profileId: string,
) {
  let sessionId = "";
  let details = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        details = sessions
          .map(
            (session) =>
              `${session.id}:${session.agent_profile_id}:${session.state}:${session.is_primary}`,
          )
          .join(", ");
        const session = sessions.find((item) => item.agent_profile_id === profileId);
        sessionId = session?.id ?? "";
        return session?.state === "WAITING_FOR_INPUT";
      },
      { timeout: 30_000, message: `profile ${profileId} never became answerable` },
    )
    .toBe(true)
    .catch((error: Error) => {
      throw new Error(`${error.message}; last observed sessions: ${details}`);
    });
  return sessionId;
}
