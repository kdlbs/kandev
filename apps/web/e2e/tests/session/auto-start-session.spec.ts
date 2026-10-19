import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("Auto-start session on task page", () => {
  test("navigating to a task with no session auto-starts one", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const reply = "auto-start-reply-sentinel";
    const prompt = `e2e:message("${reply}")`;
    const task = await apiClient.createTask(seedData.workspaceId, "Auto Start Session Task", {
      description: prompt,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      agent_profile_id: seedData.agentProfileId,
    });
    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(0);

    const automaticEnsure = watchWs(testPage).waitForResponse("session.ensure", {
      timeout: 45_000,
    });
    const [ensured] = await Promise.all([automaticEnsure, testPage.goto(`/t/${task.id}`)]);
    expect(ensured.payload).toMatchObject({
      task_id: task.id,
      session_id: expect.any(String),
      newly_created: true,
      source: "created_start",
    });
    const sessionId = ensured.payload.session_id as string;
    await waitForSessionDone(
      apiClient,
      task.id,
      sessionId,
      "Automatic first turn should finish",
      45_000,
    );
    expect((await apiClient.listTaskSessions(task.id)).sessions.map((row) => row.id)).toEqual([
      sessionId,
    ]);
    const { messages } = await apiClient.listSessionMessages(sessionId);
    expect(
      messages
        .filter((message) => message.author_type === "user")
        .map((message) => message.content),
    ).toEqual([prompt]);
    expect(
      messages
        .filter((message) => message.author_type === "agent")
        .map((message) => message.content),
    ).toContain(reply);

    // Observe the original page without reloads or recovery clicks.
    const session = new SessionPage(testPage);
    await expect(session.activeChat()).toBeVisible({ timeout: 45_000 });
    await expect(
      session.activeChat().getByTestId("agent-message-highlight").filter({ hasText: reply }),
    ).toBeVisible({ timeout: 45_000 });
    await expect(session.anyIdleInput()).toBeVisible({ timeout: 45_000 });
    expect((await apiClient.listTaskSessions(task.id)).sessions.map((row) => row.id)).toEqual([
      sessionId,
    ]);
  });
});
