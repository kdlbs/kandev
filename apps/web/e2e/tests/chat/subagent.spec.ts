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
    // Assert exactly one so an accidental duplicate render fails the test
    // rather than being masked by .first().
    const cards = session.chat.locator('[data-testid="subagent-card"]');
    await expect(cards).toHaveCount(1);
    const card = cards.first();
    await expect(card).toBeVisible();

    // Type badge and description (visible while collapsed). Scope chip queries
    // to the single card so Playwright strict mode catches duplicates instead
    // of silently selecting the first match.
    await expect(card.locator('[data-testid="subagent-type"]')).toContainText("general-purpose");
    await expect(card.locator('[data-testid="subagent-description"]')).toContainText(
      "Explore the codebase",
    );

    // Metadata row of chips surfaces the completed subagent's metrics.
    await expect(card.locator('[data-testid="subagent-meta"]')).toBeVisible();
    await expect(card.locator('[data-testid="subagent-meta-duration"]')).toContainText("2.2s");
    await expect(card.locator('[data-testid="subagent-meta-tokens"]')).toContainText("9,987");
    await expect(card.locator('[data-testid="subagent-meta-tools"]')).toContainText("3 tools");
    // The agent id is truncated with an ellipsis, so assert on a substring.
    await expect(card.locator('[data-testid="subagent-meta-agent"]')).toContainText("agent_e2e");
  });
});
