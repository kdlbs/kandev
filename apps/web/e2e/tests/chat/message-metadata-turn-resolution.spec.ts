// Regression guard: the chat message "Message Metadata" dialog must resolve
// `turn_metadata` for messages in sessions the client did NOT hydrate at page
// load. The boot/SSR state hydrates turns only for the page-load active
// session; switching to another session fetched its messages but never its
// turns, so every message of that session rendered `turn_metadata: null`
// even though the turn exists server-side with metadata (the dialog's field
// became reachable with #2683, which exposed the gap).
import { test, expect, type Page } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForStableActiveSession } from "../../helpers/session-store";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Creates a task with two REAL mock-agent sessions (launched through the
 * normal agent flow so both get task environments and turns with metadata).
 * Session 1 is the task's primary session, so SSR hydrates its turns; session
 * 2 is the non-hydrated session the test switches to.
 */
async function createTaskWithTwoSessions(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ session1Id: string; session2Id: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Turn Metadata Resolution",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  // First session must reach a done state before we open the second.
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 30_000, message: "Waiting for first session to finish" },
    )
    .toBe(true);

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  const card = kanban.taskCardByTitle("Turn Metadata Resolution");
  await expect(card).toBeVisible({ timeout: 10_000 });
  await card.click();
  await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
    timeout: 15_000,
  });

  // Launch the second session via the new-session dialog.
  await session.openNewSessionDialog();
  await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
  await session.newSessionPromptInput().fill("/e2e:simple-message");
  await session.newSessionStartButton().click();
  await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
      },
      { timeout: 60_000, message: "Waiting for both sessions to finish" },
    )
    .toBe(2);

  const { sessions } = await apiClient.listTaskSessions(task.id);
  const sorted = sessions.sort(
    (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
  );
  return { session1Id: sorted[0].id, session2Id: sorted[1].id };
}

test.describe("message metadata turn resolution across sessions", () => {
  test("turn_metadata resolves after switching to a non-hydrated session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session1Id, session2Id } = await createTaskWithTwoSessions(
      testPage,
      apiClient,
      seedData,
    );

    // Reload so the store is rebuilt from scratch. SSR hydrates turns only
    // for the task's PRIMARY session (session 1, auto-created at task
    // creation); session 2's turns were delivered over WS while the page was
    // open and are wiped by the reload — the exact "turns missing from the
    // store" state the bug report hits.
    await testPage.reload({ waitUntil: "domcontentloaded" });
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.showSessionContext();

    // Both sessions render as sibling tabs (the sibling panel is created
    // asynchronously by the auto-session-tab effect after the layout settles).
    await expect(session.sessionTabBySessionId(session1Id)).toBeVisible({ timeout: 15_000 });
    await expect(session.sessionTabBySessionId(session2Id)).toBeVisible({ timeout: 15_000 });

    // Make the primary session active first (idempotent when the restored
    // layout already selected it), so the switch below is a real transition.
    await session.sessionTabBySessionId(session1Id).click();
    await waitForStableActiveSession(testPage, session1Id);

    // Switch to the non-primary session — its chat mounts and fetches
    // messages, and (with the fix) its turns must be fetched and merged into
    // the store. Pre-fix every message of this session resolves to
    // `turn = null`, so the dialog renders `turn_metadata: null`.
    await session.sessionTabBySessionId(session2Id).click();
    await waitForStableActiveSession(testPage, session2Id);

    const chat = session.activeChat();
    await expect(chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Open the metadata dialog on session 2's agent message.
    const messageBody = chat
      .locator("[data-agent-message-body][data-message-id]")
      .filter({ hasText: "simple mock response" });
    const actionsRow = messageBody.locator("xpath=..");
    await actionsRow.getByRole("button", { name: "Show message metadata" }).click();

    const dialog = testPage.locator('[data-slot="dialog-content"]');
    await expect(dialog).toBeVisible();

    // RED assertion: turn_metadata must be populated, not null. The dialog
    // renders the value verbatim; a missing turn renders the literal `null`.
    await expect(
      dialog.locator("pre").filter({ hasText: /runtime_config_snapshot/ }),
    ).toBeVisible();
  });
});
