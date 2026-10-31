// AC-UI-INBOX-HISTORY-001.29: end-to-end coverage for the History tab --
// selecting it, observing a `superseded` bundle with its asked time, the
// text of the question it actually asked (AC .30), and its
// step-starts-no-agent label (AC .11, named by that concept, never
// "parked") -- and confirming no answer affordance is present (AC .14).
import { test, expect } from "../../fixtures/test-base";
import {
  expectClarificationReferencePopup,
  expectTruncatedQuestionSummary,
  seedMarkdownHistory,
} from "../../helpers/clarification-markdown";

const QUESTION_TITLE = "Pick a deployment target";
const QUESTION_PROMPT = "Which environment should this ship to?";
const OPTION_LABEL = "Staging";

test.describe("Inbox History tab", () => {
  // @covers AC-UI-CLARIFICATION-MARKDOWN-001.1 through .4, .6, .7
  test("renders Markdown question history with independent reference links", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const fixture = await seedMarkdownHistory(apiClient, seedData, "Markdown question history");
    try {
      await testPage.goto("/needs-you-inbox");
      await testPage.getByTestId("inbox-tab-history").click();
      const row = testPage.locator(`[data-pending-id="${fixture.pendingId}"]`);
      await expect(row).toHaveCount(1);
      const toggle = row.getByTestId("inbox-history-row-toggle");
      await expect(toggle.locator("code")).toHaveCount(1);
      await expect(toggle).toContainText("Read storage guide");
      await expect(toggle.locator("a")).toHaveCount(0);
      await expectTruncatedQuestionSummary(toggle.locator("span.font-medium"));
      await toggle.click();

      const detail = row.getByTestId("inbox-history-clarification-detail");
      await expect(detail.getByText("pnpm build pnpm test", { exact: true })).toHaveCSS(
        "white-space",
        "pre-wrap",
      );
      await expect(detail.locator("ol > li")).toHaveCount(2);
      await expect(detail.locator("strong")).toHaveText("retention");
      await expect(detail.locator("code").filter({ hasText: "Postgres" })).toBeVisible();
      const guidance = detail.getByRole("link", { name: "storage guide", exact: true });
      await expect(guidance).toHaveAttribute("rel", "noopener noreferrer");
      await toggle.press("Tab");
      await testPage.keyboard.press("Tab");
      await testPage.keyboard.press("Tab");
      await expect(guidance).toBeFocused();
      await expectClarificationReferencePopup(testPage, () => guidance.press("Enter"));
      await expect(toggle).toHaveAttribute("aria-expanded", "true");
      await prCapture.screenshot("desktop-clarification-markdown-history", {
        caption: "History renders Markdown questions with independent reference links",
      });

      const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
      const original = messages.find(
        (message) => message.metadata?.pending_id === fixture.pendingId,
      );
      expect(original?.metadata?.question).toEqual(fixture.question);
      expect(original?.metadata?.status).toBe("pending");
      await expect(row.getByRole("button", { name: /^answer|dismiss|snooze/i })).toHaveCount(0);
    } finally {
      await apiClient.deleteTask(fixture.taskId);
    }
  });

  test("lists a superseded question with its asked time, question text and step label, with no answer affordance (AC .29)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Keep the extra step in a temporary workflow. The seeded workflow is
    // shared by later tests in this worker, so mutating it would leak an
    // additional row into workflow-stepper assertions.
    const historyWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Inbox History Workflow ${Date.now()}`,
    );

    // A step whose on_enter carries no auto_start_agent entry, so the row
    // must render the AC .11 step-starts-no-agent label.
    const noAgentStep = await apiClient.createWorkflowStep(
      historyWorkflow.id,
      "Inbox History No-Agent Step",
      9,
      { events: { on_enter: [] } },
    );

    const title = "Inbox History Superseded Flow";
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: historyWorkflow.id,
      workflow_step_id: noAgentStep.id,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
    });

    const turnAStartedAt = new Date(Date.now() - 120_000).toISOString();
    const turnBStartedAt = new Date(Date.now() - 60_000).toISOString();

    // AC .8's real sequence: ask a question on turn A, then start turn B
    // without ever answering it. Turn A is completed (the agent's turn ends
    // blocked on the question) so turn B, which starts later, wins current-
    // turn authority outright -- exercising `superseded`, not a shadowed tie.
    await apiClient.seedSessionMessage(sessionId, {
      type: "clarification_request",
      newTurn: true,
      turnStartedAt: turnAStartedAt,
      turnCompletedAt: turnAStartedAt,
      metadata: {
        pending_id: "pend-history-superseded-1",
        session_id: sessionId,
        question_id: "q1",
        question_index: 0,
        question_total: 1,
        status: "pending",
        question: {
          id: "q1",
          title: QUESTION_TITLE,
          prompt: QUESTION_PROMPT,
          options: [{ option_id: "opt-staging", label: OPTION_LABEL, description: "" }],
        },
      },
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "Starting a different approach.",
      newTurn: true,
      turnStartedAt: turnBStartedAt,
    });

    await testPage.goto("/needs-you-inbox");
    await testPage.getByTestId("inbox-tab-history").click();

    const row = testPage.getByTestId("inbox-history-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 30_000 });

    await expect(row.getByTestId("inbox-history-row-reason")).toContainText(/superseded/i);
    await expect(row.getByTestId("inbox-history-row-asked-time")).not.toBeEmpty();

    await row.getByTestId("inbox-history-row-toggle").click();
    const detail = row.getByTestId("inbox-history-clarification-detail");
    await expect(detail).toBeVisible();
    await expect(detail).toContainText(QUESTION_TITLE);
    await expect(detail).toContainText(QUESTION_PROMPT);
    await expect(detail).toContainText(OPTION_LABEL);

    await expect(row.getByTestId("inbox-history-step-starts-no-agent")).toBeVisible();

    // Read-only: opening the task and copying the identifier are the only
    // actions, and none of them answers, dismisses or snoozes anything.
    await expect(row.getByTestId("inbox-history-open-task")).toBeVisible();
    await expect(row.getByTestId("inbox-history-copy-id")).toBeVisible();
    await expect(row.getByRole("button", { name: /^answer/i })).toHaveCount(0);
    await expect(row.getByRole("button", { name: /dismiss/i })).toHaveCount(0);
    await expect(row.getByRole("button", { name: /snooze/i })).toHaveCount(0);
  });
});
