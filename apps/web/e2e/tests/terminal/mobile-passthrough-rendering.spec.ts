// Routing: /t/{taskId}. File name starts with "mobile-" so it runs on the
// mobile-chrome Playwright project (Pixel 5 emulation).
//
// Regression for #1031: mobile Chat tab must render PassthroughTerminal when
// session.is_passthrough is set even if agent_profile_snapshot is absent from
// the client store (lean list responses / partial WS state_changed events).
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { stripSessionProfileSnapshot } from "../../helpers/session-store";

async function createTUIProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((a) => a.name === "mock-agent");
  if (!mockAgent) {
    throw new Error(`mock-agent not found (got ${agents.map((a) => a.name).join(", ")})`);
  }
  return apiClient.createAgentProfile(mockAgent.id, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
}

test.describe("Mobile passthrough rendering", () => {
  test("keeps PassthroughTerminal on Chat when snapshot is stripped from store", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profile = await createTUIProfile(apiClient, "Mobile Passthrough");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Passthrough Task",
      profile.id,
      {
        description: "hello from mobile passthrough e2e",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const sessionId = task.session_id;
    if (!sessionId) {
      throw new Error("createTaskWithAgent did not return session_id");
    }

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);

    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();
    await session.expectPassthroughHasText("Mock Agent");

    await stripSessionProfileSnapshot(testPage, sessionId);

    // Switch away from Chat and back — mirrors the user flow that re-evaluates
    // isPassthroughMode from the (now snapshot-less) session row.
    await testPage.getByRole("button", { name: "Plan" }).tap();
    await testPage.getByRole("button", { name: "Chat" }).tap();

    await expect(session.passthroughTerminal).toBeVisible({ timeout: 15_000 });
    await expect(session.chat).toBeHidden();
    await session.expectPassthroughHasText("Mock Agent");
  });
});
