// Chat message pagination — "Load older messages" button reliably walks a long
// conversation all the way back to the very first message (the initial prompt).
//
// Regression for the scroll-up lazy-loader wedging at the top: the
// IntersectionObserver only fires on intersection *transitions*, so once the
// user is pinned at scrollTop 0 with the sentinel permanently in view it never
// re-arms and older messages stop loading. The explicit button does not depend
// on scroll/intersection state, so it always makes progress.
//
// Covers the native renderer (the production default — `STRATEGY = "native"`).
// The virtuoso renderer shares the same MessageListStatus button, but its
// windowed rendering keeps off-screen items out of the DOM, so asserting the
// oldest message is *visible* there is a virtualization concern, not a
// pagination one — out of scope for this regression.
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedMessagesDescription } from "../search/shared";

// Stands in for the user's "initial prompt": an early chat message seeded near
// the start of the conversation, then buried under a large turn so it falls
// outside the initial newest-100 fetch window. Kept OUT of the task description
// (which the chat renders verbatim) so the only on-screen occurrence is the
// paginated message itself.
const INITIAL_PROMPT = "INITIAL-PROMPT-MARKER-7Q2X";
// Enough filler to require multiple explicit load-older pages beyond the
// initial 100. The native top sentinel may opportunistically fetch one or two
// older pages before the first assertion on fast layouts, so keep the marker
// well behind that automatic head start.
const FILLER_COUNT = 220;

/** Boot an idle session, seed the marker, then bury it under FILLER_COUNT messages. */
async function seedBigConversation(apiClient: ApiClient, seedData: SeedData): Promise<string> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "message-pagination-load-older",
    seedData.agentProfileId,
    {
      description: seedMessagesDescription(["chat ready"]),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sessionId = task.session_id!;
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some((m) => m.content.includes("chat ready"));
      },
      { timeout: 60_000, message: "Waiting for the boot turn to persist" },
    )
    .toBe(true);
  await apiClient.seedSessionMessage(sessionId, { type: "message", content: INITIAL_PROMPT });
  await apiClient.seedAgentMessages(sessionId, FILLER_COUNT);
  return task.id;
}

test.describe("@chat message pagination", () => {
  test("load-older button reaches the initial prompt in a big conversation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const taskId = await seedBigConversation(apiClient, seedData);

    // Open the session fresh (SSR + initial fetch loads only the newest ~100).
    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    // Filler renders; the initial prompt does NOT yet (older than the window).
    await expect(session.chat.getByText("filler message", { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.chat.getByText(INITIAL_PROMPT)).toHaveCount(0);

    // The explicit load-older button is offered because more messages exist.
    const loadOlder = session.chat.getByTestId("load-older-messages");
    await expect(loadOlder).toBeVisible({ timeout: 10_000 });

    // Click it repeatedly until the initial prompt is reached. The button is the
    // reliable path — no scrolling, no intersection timing.
    await expect
      .poll(
        async () => {
          if (
            await session.chat
              .getByText(INITIAL_PROMPT)
              .isVisible()
              .catch(() => false)
          ) {
            return "reached";
          }
          if (await loadOlder.isVisible().catch(() => false)) {
            await loadOlder.click().catch(() => undefined);
          }
          return "loading";
        },
        { timeout: 60_000, intervals: [300], message: "Loading older pages until initial prompt" },
      )
      .toBe("reached");

    // The initial prompt is visible, and there's nothing left to load.
    await expect(session.chat.getByText(INITIAL_PROMPT)).toBeVisible();
    await expect(loadOlder).toBeHidden();
  });
});
