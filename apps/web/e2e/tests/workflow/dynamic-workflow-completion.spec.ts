import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function createDynamicProfile(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
  candidateProfileId: string,
) {
  const response = await apiClient.rawRequest("POST", "/api/v1/agents/dynamic/profiles", {
    name: "Dynamic workflow completion",
    dynamic: {
      version: 1,
      candidates: [
        {
          position: 0,
          execution_profile_id: candidateProfileId,
          enabled: true,
        },
      ],
    },
  });
  if (!response.ok) {
    throw new Error(
      `Dynamic profile creation failed (${response.status}): ${await response.text()}`,
    );
  }
  return (await response.json()) as { id: string };
}

async function waitForAgentMarker(
  apiClient: InstanceType<typeof import("../../helpers/api-client").ApiClient>,
  sessionId: string,
  marker: string,
) {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "agent" && message.content.includes(marker),
        );
      },
      { timeout: 45_000, message: `session ${sessionId} did not receive ${marker}` },
    )
    .toBe(true);
}

test.describe("Dynamic workflow completion", () => {
  test("reuses the logical session after completion and reload", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });
    let dynamicProfileId = "";

    try {
      const dynamicProfile = await createDynamicProfile(apiClient, seedData.agentProfileId);
      dynamicProfileId = dynamicProfile.id;
      const workflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "Dynamic completion reuse",
      );
      const source = await apiClient.createWorkflowStep(workflow.id, "Implement", 0, {
        is_start_step: true,
        agent_profile_id: dynamicProfile.id,
        profile_session_start_policy: "reuse",
        profile_session_end_policy: "park",
        events: { on_turn_complete: [{ type: "move_to_next" }] },
      });
      const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1, {
        agent_profile_id: dynamicProfile.id,
        profile_session_start_policy: "reuse",
        profile_session_end_policy: "park",
      });
      const firstMarker = "dynamic-completion-first-turn";
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Dynamic completion reuse task",
        dynamicProfile.id,
        {
          description: `e2e:message("${firstMarker}")`,
          workflow_id: workflow.id,
          workflow_step_id: source.id,
          repository_ids: [seedData.repositoryId],
        },
      );

      let sessionId = "";
      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            const session = sessions.find(
              (candidate) => candidate.agent_profile_id === dynamicProfile.id,
            );
            sessionId = session?.id ?? "";
            return session?.state === "WAITING_FOR_INPUT";
          },
          { timeout: 45_000, message: "Dynamic workflow session did not become ready" },
        )
        .toBe(true);
      await waitForAgentMarker(apiClient, sessionId, firstMarker);
      await expect
        .poll(() => apiClient.getTask(task.id).then((current) => current.workflow_step_id), {
          timeout: 30_000,
          message: "completed turn did not advance to Review",
        })
        .toBe(review.id);

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await testPage.reload();
      await session.waitForLoad();
      await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step");
      await session.sendMessage('e2e:message("dynamic-completion-second-turn")');
      await waitForAgentMarker(apiClient, sessionId, "dynamic-completion-second-turn");

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            const completedSession = sessions.find((candidate) => candidate.id === sessionId);
            return completedSession
              ? {
                  id: completedSession.id,
                  agent_profile_id: completedSession.agent_profile_id,
                  is_primary: completedSession.is_primary,
                  state: completedSession.state,
                }
              : null;
          },
          {
            timeout: 45_000,
            message: "completed second turn did not settle on the reused session",
          },
        )
        .toEqual({
          id: sessionId,
          agent_profile_id: dynamicProfile.id,
          is_primary: true,
          state: "WAITING_FOR_INPUT",
        });

      const finalSessions = (await apiClient.listTaskSessions(task.id)).sessions;
      expect(finalSessions).toHaveLength(1);
      expect(finalSessions[0]).toMatchObject({
        id: sessionId,
        agent_profile_id: dynamicProfile.id,
        is_primary: true,
        state: "WAITING_FOR_INPUT",
      });
      expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(review.id);
      await expect(session.activeChat()).toContainText("dynamic-completion-second-turn");
    } finally {
      try {
        if (dynamicProfileId) await apiClient.deleteAgentProfile(dynamicProfileId, true);
      } finally {
        await releaseFeature();
      }
    }
  });
});
