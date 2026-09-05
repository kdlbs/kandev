import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function waitForPersistedSubagentMetrics(
  apiClient: {
    listSessionMessages: (sessionId: string) => Promise<{
      messages: Array<{ metadata?: Record<string, unknown> }>;
    }>;
  },
  sessionId: string,
) {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some((message) => {
          const normalized = message.metadata?.normalized;
          if (!normalized || typeof normalized !== "object") return false;
          const subagentTask = (normalized as { subagent_task?: unknown }).subagent_task;
          if (!subagentTask || typeof subagentTask !== "object") return false;
          const metrics = subagentTask as {
            duration_ms?: unknown;
            total_tokens?: unknown;
            tool_use_count?: unknown;
          };
          return (
            metrics.duration_ms === 2200 &&
            metrics.total_tokens === 9987 &&
            metrics.tool_use_count === 3
          );
        });
      },
      {
        timeout: 30_000,
        message: "subagent completion metrics were not persisted in the session",
      },
    )
    .toBe(true);
}

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

    // Chat idle means the outer turn is no longer streaming. The nested
    // subagent completion event can still be hydrating into the card. Wait for
    // the persisted message that causes the metadata row before checking the
    // rendered consequence.
    const sessionId = task.session_id ?? task.primary_session_id;
    expect(sessionId).toBeTruthy();
    if (!sessionId) throw new Error("agent task did not return a session id");
    await waitForPersistedSubagentMetrics(apiClient, sessionId);

    const metadata = card.locator('[data-testid="subagent-meta"]');
    // A live update may have been missed while the page was hydrating. Once
    // the backend state is present, one reload rehydrates the authoritative
    // message list without turning this into an unbounded retry loop.
    if (!(await metadata.isVisible())) {
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 15_000 });
    }
    await expect(metadata).toBeVisible();
    // Successful completion: identity chips stay, Working is gone, a check remains.
    await expect(card.getByRole("status", { name: "Loading" })).toHaveCount(0);
    await expect(card.getByText("Working...")).toHaveCount(0);
    await expect(card.getByLabel("Completed")).toHaveCount(1);
    await expect(card.locator('[data-testid="subagent-meta-duration"]')).toContainText("2.2s");
    await expect(card.locator('[data-testid="subagent-meta-tokens"]')).toContainText("9,987");
    await expect(card.locator('[data-testid="subagent-meta-tools"]')).toContainText("3 tools");

    // The subagent's internal tool call is nested UNDER its card (the mock emits
    // a child carrying `_meta.claudeCode.parentToolUseId`), not as a flat sibling.
    await expect(card.locator('[data-testid="subagent-child-count"]')).toContainText("1 tool call");
    // Expand the card and confirm the child (sleep 30) renders inside it.
    await card.getByRole("button").first().click();
    await expect(card).toContainText("sleep 30");
  });
});
