import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// Drives the mock-agent's `subagent` scenario (triggered by the `/e2e:subagent`
// directive). The scenario emits a claude-style subagent (Task) tool call with
// full result metadata, which the kandev adapter normalizes to a subagent_task
// payload. This file asserts the dedicated subagent card renders in the chat
// with its type badge, description, and metadata chips. The card auto-collapses
// once the subagent completes, so the badge/description/meta row are visible
// without expanding.

test.describe("Subagent card", () => {
  test("renders type badge, description, and metadata chips on completion", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Subagent card",
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

    // The dedicated subagent card renders, not the generic tool_call row.
    const card = session.chat.locator('[data-testid="subagent-card"]').first();
    await expect(card).toBeVisible();

    // Type badge and description (visible while collapsed).
    await expect(session.chat.locator('[data-testid="subagent-type"]').first()).toContainText(
      "general-purpose",
    );
    await expect(
      session.chat.locator('[data-testid="subagent-description"]').first(),
    ).toContainText("Explore the codebase");

    // Metadata row of chips surfaces the completed subagent's metrics.
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
    // The agent id is truncated with an ellipsis, so assert on a substring.
    await expect(session.chat.locator('[data-testid="subagent-meta-agent"]').first()).toContainText(
      "agent_e2e",
    );
  });
});
