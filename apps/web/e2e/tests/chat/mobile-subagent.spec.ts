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

    // Assert exactly one card so a duplicate render fails rather than hiding behind .first().
    const cards = session.chat.locator('[data-testid="subagent-card"]');
    await expect(cards).toHaveCount(1);
    const card = cards.first();
    await expect(card).toBeVisible();

    // Scope chip queries to the single card (strict mode catches duplicates).
    await expect(card.locator('[data-testid="subagent-type"]')).toContainText("general-purpose");
    await expect(card.locator('[data-testid="subagent-description"]')).toContainText(
      "Explore the codebase",
    );

    // Chat idle can arrive before the nested subagent completion event is
    // hydrated into the card. Wait for the metadata state boundary explicitly.
    await expect(card.locator('[data-testid="subagent-meta"]')).toBeVisible({ timeout: 15_000 });
    // Successful completion: identity chips stay, Working is gone, a check remains.
    await expect(card.getByRole("status", { name: "Loading" })).toHaveCount(0);
    await expect(card.getByText("Working...")).toHaveCount(0);
    await expect(card.getByLabel("Completed")).toHaveCount(1);
    await expect(card.locator('[data-testid="subagent-meta-duration"]')).toContainText("2.2s");
    await expect(card.locator('[data-testid="subagent-meta-tokens"]')).toContainText("9,987");
    await expect(card.locator('[data-testid="subagent-meta-tools"]')).toContainText("3 tools");

    // The subagent's internal tool call nests under its card (parentToolUseId).
    await expect(card.locator('[data-testid="subagent-child-count"]')).toContainText("1 tool call");
  });
});
