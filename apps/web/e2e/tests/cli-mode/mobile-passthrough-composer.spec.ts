// Filename starts with "mobile-" so this runs in the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { seedAvailableCommands } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";

async function createPassthroughProfile(apiClient: ApiClient, name: string): Promise<string> {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
  const profile = await apiClient.createAgentProfile(agents[0].id, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
  return profile.id;
}

test.describe("mobile CLI mode: passthrough composer", () => {
  test("slash command suggestions can be selected by touch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "Mobile CLI Commands");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile CLI Commands Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
    await session.expectPassthroughHasText("Processed:", 20_000);
    await seedAvailableCommands(testPage, task.session_id, [
      { name: "slow", description: "Run slowly" },
      { name: "error", description: "Trigger an error" },
    ]);

    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const textarea = testPage.getByTestId("passthrough-composer-textarea");
    await expect(textarea).toBeVisible({ timeout: 5_000 });

    await textarea.fill("/s");

    await expect(testPage.getByRole("listbox", { name: "Command suggestions" })).toBeVisible({
      timeout: 10_000,
    });
    await testPage.getByRole("option", { name: "/slow" }).tap();

    await expect(textarea).toHaveValue("/slow ");
    await expect(testPage.getByTestId("passthrough-composer-suggestions")).toHaveCount(0);
  });
});
