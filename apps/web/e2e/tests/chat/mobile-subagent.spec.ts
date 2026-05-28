import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// Mobile (Pixel 5) coverage for the subagent card — same `/e2e:subagent`
// scenario as the desktop spec. Filename matches /mobile-.*\.spec\.ts/ so the
// `mobile-chrome` project picks it up. Asserts the card is visible and usable
// (type badge, description, and metadata chips all readable) at mobile width.

test.describe("Mobile subagent card", () => {
  test("renders type badge, description, and metadata chips at mobile width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile subagent card",
      seedData.agentProfileId,
      {
        description: "/e2e:subagent",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const card = session.chat.locator('[data-testid="subagent-card"]').first();
    await expect(card).toBeVisible();

    await expect(session.chat.locator('[data-testid="subagent-type"]').first()).toContainText(
      "general-purpose",
    );
    await expect(
      session.chat.locator('[data-testid="subagent-description"]').first(),
    ).toContainText("Explore the codebase");

    await expect(session.chat.locator('[data-testid="subagent-meta"]').first()).toBeVisible();
    await expect(
      session.chat.locator('[data-testid="subagent-meta-duration"]').first(),
    ).toContainText("2.2s");
    await expect(
      session.chat.locator('[data-testid="subagent-meta-tokens"]').first(),
    ).toContainText("9,987");
    await expect(session.chat.locator('[data-testid="subagent-meta-tools"]').first()).toContainText(
      "3 tools",
    );
    await expect(session.chat.locator('[data-testid="subagent-meta-agent"]').first()).toContainText(
      "agent_e2e",
    );
  });
});
