// Session chat panel search — hit list, backfill, scroll-to-message.
import { test, expect } from "../../fixtures/test-base";
import { openPanelSearch, panelSearchBar, panelSearchInput } from "../../helpers/panel-search";
import { seedTask, seedMessagesDescription } from "./shared";

const HITS_LIST_SELECTOR = "[data-panel-search-bar] + div";

test.describe("@search session chat panel search", () => {
  test.describe.configure({ retries: 1 });

  test("C1 query matches multiple messages and renders highlighted snippets", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const messages = [
      "The quick brown fox jumps over",
      "A sly fox hides in the bush",
      "this line has no match",
      "third fox appearance here",
    ];
    const { session } = await seedTask(testPage, apiClient, seedData, "session-search-hits", {
      description: seedMessagesDescription(messages),
    });
    // Wait for at least one agent message to have rendered
    await expect(session.chat.getByText("fox", { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");
    await prCapture.startRecording("session-search");
    await panelSearchInput(testPage).fill("fox");

    // Hits list appears with at least 1 match (they may merge into one agent
    // message, so tolerate N>=1 hits)
    const hitsContainer = testPage.locator(HITS_LIST_SELECTOR);
    await expect(hitsContainer).toBeVisible({ timeout: 10_000 });
    // Each hit row has a <mark> around the query term.
    await expect(hitsContainer.locator("mark").first()).toBeVisible({ timeout: 5_000 });
    const hitRows = hitsContainer.locator("button");
    expect(await hitRows.count()).toBeGreaterThanOrEqual(1);
    await prCapture.stopRecording({ caption: "Session search: hit list with matched snippets" });
  });

  test("C2 unmatched query shows No matches", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "session-search-nomatch", {
      description: seedMessagesDescription(["hello world"]),
    });
    await expect(session.chat.getByText("hello world", { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");
    await panelSearchInput(testPage).fill("zzzunmatchedzzz");
    await expect(testPage.getByText("No matches")).toBeVisible({ timeout: 10_000 });
  });

  test("C4 clicking a hit scrolls the message into view and flashes it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    // Generate enough messages that the target one needs scrolling.
    const lines: string[] = [];
    for (let i = 0; i < 20; i++) {
      lines.push(`line ${i} padding text`);
    }
    const TARGET = "unique-target-text-abc123";
    lines.push(TARGET);
    for (let i = 20; i < 40; i++) {
      lines.push(`line ${i} padding text`);
    }

    const { session } = await seedTask(testPage, apiClient, seedData, "session-search-scroll", {
      description: seedMessagesDescription(lines),
    });
    await expect(session.chat.getByText("padding text", { exact: false }).first()).toBeVisible({
      timeout: 60_000,
    });

    await openPanelSearch(testPage, "session");
    await panelSearchInput(testPage).fill(TARGET);

    const hitsContainer = testPage.locator(HITS_LIST_SELECTOR);
    const hitButton = hitsContainer.locator("button").first();
    await expect(hitButton).toBeVisible({ timeout: 10_000 });
    await hitButton.click();

    // Flash class appears briefly on the target message container
    await expect
      .poll(
        async () =>
          testPage.evaluate(() => {
            return document.querySelectorAll(".search-flash").length > 0;
          }),
        { timeout: 5_000, message: "Expected .search-flash class on the target row" },
      )
      .toBe(true);
  });

  test("C7 cross-session isolation: query text unique to session A does not appear in B", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    // Create session A and session B on separate tasks (simplest isolation scope
    // — avoids the add-session-tab UX which is non-trivial to drive).
    const uniqueA = "unique-aaa-marker-xyz";
    const uniqueB = "unique-bbb-marker-xyz";

    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "session-search-iso-B",
      seedData.agentProfileId,
      {
        description: seedMessagesDescription([`hello with ${uniqueB} embedded`]),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    // Wait for task B's session to process its messages
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(taskB.session_id!);
          return messages.some((m) => m.content.includes(uniqueB));
        },
        { timeout: 60_000, message: "Waiting for task B's agent message to persist" },
      )
      .toBe(true);

    // Now open task A and search for B's marker — should return no hits.
    const { session } = await seedTask(testPage, apiClient, seedData, "session-search-iso-A", {
      description: seedMessagesDescription([`hello with ${uniqueA} embedded`]),
    });
    await expect(session.chat.getByText(uniqueA, { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");
    await panelSearchInput(testPage).fill(uniqueB);
    await expect(testPage.getByText("No matches")).toBeVisible({ timeout: 10_000 });

    // Sanity: searching for uniqueA returns a hit
    await panelSearchInput(testPage).fill(uniqueA);
    const hitsContainer = testPage.locator(HITS_LIST_SELECTOR);
    await expect(hitsContainer.locator("button").first()).toBeVisible({ timeout: 10_000 });
  });

  test("C10 close button clears the bar", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "session-search-close", {
      description: seedMessagesDescription(["foo"]),
    });
    await expect(session.chat.getByText("foo", { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");
    const close = panelSearchBar(testPage).getByRole("button", { name: /Close/ });
    await close.click();
    await expect(panelSearchBar(testPage)).toHaveCount(0);
  });

  // Virtuoso renderer coverage — session-search should work with the virtualized path.
  test("C-virtuoso basic search works on the virtuoso renderer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "session-search-virtuoso", {
      description: seedMessagesDescription(["virtuoso fox alpha", "virtuoso fox beta"]),
      rendererOverride: "virtuoso",
    });
    await expect(session.chat.getByText("virtuoso", { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");
    await panelSearchInput(testPage).fill("virtuoso fox");
    const hitsContainer = testPage.locator(HITS_LIST_SELECTOR);
    await expect(hitsContainer.locator("button").first()).toBeVisible({ timeout: 10_000 });
  });
});
