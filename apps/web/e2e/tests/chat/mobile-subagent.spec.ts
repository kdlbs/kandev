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
    await card.getByRole("button").first().click();
    await expect(card).toContainText("sleep 30");
  });

  test("renders control labels at mobile width", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile subagent controls",
      seedData.agentProfileId,
      {
        description: "/e2e:subagent-controls",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    await session.chat.getByRole("button", { name: /2 tool calls/ }).click();
    await expect(session.chat.getByText("Messaged subagent", { exact: true })).toBeVisible();
    await expect(session.chat.getByText("wait", { exact: true })).toBeVisible();
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow).toBe(false);
  });

  test("shows a resumed background subagent count after SendMessage", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Resume a background subagent on mobile",
      seedData.agentProfileId,
      {
        description: "/e2e:subagent-resume-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const sessionId = task.session_id ?? task.primary_session_id;
    expect(sessionId).toBeTruthy();
    if (!sessionId) throw new Error("agent task did not return a session id");

    const cards = session.chat.locator('[data-testid="subagent-card"]');
    await expect(cards).toHaveCount(1);
    const card = cards.first();
    await expect(card.locator('[data-testid="subagent-meta-background"]')).toBeVisible();

    await session.sendMessageViaButton("/e2e:subagent-resume");
    await session.waitForChatIdle({ timeout: 30_000 });

    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some((message) => {
          const normalized = message.metadata?.normalized;
          if (!normalized || typeof normalized !== "object") return false;
          const subagentTask = (normalized as { subagent_task?: unknown }).subagent_task;
          return (
            !!subagentTask &&
            typeof subagentTask === "object" &&
            (subagentTask as { resume_count?: unknown }).resume_count === 1
          );
        });
      })
      .toBe(true);

    await expect(card.locator('[data-testid="subagent-meta-resume"]')).toContainText("Resumed x1", {
      timeout: 15_000,
    });
    await expect(card.getByLabel("Completed")).toHaveCount(1);
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow).toBe(false);
  });
});
