import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { readSessionRuntimeIdentity } from "../../helpers/session-resume-prompt-queue";

const AGENT_NAME = "E2E Restoring Acp Agent";
const AGENT_SLUG = "e2e-restoring-acp-agent";

test.describe("Custom ACP agent session restore", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.deleteCustomAgentByName(AGENT_SLUG);
  });

  // The mock agent names each new session after its own process, so a
  // restarted backend that sent session/new would report a different ID.
  // @covers AC-AGENTS-CUSTOM-ACP-002.1
  test("keeps the provider conversation across a backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const agent = await apiClient.createCustomTUIAgent({
      display_name: AGENT_NAME,
      command: "mock-agent",
      protocol: "acp",
    });
    const profile = await apiClient.createAgentProfile(agent.id, `${AGENT_NAME} profile`, {
      model: "mock-fast",
      cli_passthrough: false,
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Custom ACP session restore",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("custom ACP task has no session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.expectChatResponseVisible("simple mock response", 0, { timeout: 60_000 });
    await session.waitForChatIdle({ timeout: 30_000 });
    const before = await readSessionRuntimeIdentity(apiClient, task.id, task.session_id);

    await backend.restart();
    await testPage.reload();
    await session.waitForLoad();
    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 60_000 });

    const after = await readSessionRuntimeIdentity(apiClient, task.id, task.session_id);
    expect(after.acpSessionId).toBe(before.acpSessionId);
  });
});
