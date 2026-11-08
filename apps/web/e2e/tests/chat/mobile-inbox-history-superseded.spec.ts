// AC-UI-INBOX-HISTORY-001.14/.29: the History tab's read-only actions must
// also work on a phone-sized viewport, where the row falls back to icon-only
// AC .14 actions with the 44px touch target (control-sizing.tsx's
// coarse-pointer breakpoint) instead of the desktop text-labelled pair. This
// mirrors inbox-history-superseded.spec.ts's desktop coverage rather than
// duplicating its assertions.
import { test, expect } from "../../fixtures/test-base";
import {
  expectClarificationReferencePopup,
  expectTruncatedQuestionSummary,
  LONG_REFERENCE_LABEL,
  seedMarkdownHistory,
} from "../../helpers/clarification-markdown";

const MIN_TOUCH_TARGET_PX = 44;
const QUESTION_TITLE = "Pick a deployment target";
const QUESTION_PROMPT = "Which environment should this ship to?";
const OPTION_LABEL = "Staging";

test.describe("Inbox History tab on mobile", () => {
  // @covers AC-UI-CLARIFICATION-MARKDOWN-001.1 through .3, .5 through .7
  test("renders Markdown question history without overflow", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const fixture = await seedMarkdownHistory(apiClient, seedData, "Mobile Markdown history");
    try {
      await testPage.goto("/needs-you-inbox");
      await testPage.getByTestId("inbox-tab-history").tap();
      const row = testPage.locator(`[data-pending-id="${fixture.pendingId}"]`);
      await expect(row).toHaveCount(1);
      const toggle = row.getByTestId("inbox-history-row-toggle");
      await expect(toggle).toContainText("Read storage guide");
      await expectTruncatedQuestionSummary(toggle.locator("span.font-medium"));
      await toggle.tap();
      const detail = row.getByTestId("inbox-history-clarification-detail");
      await expect(detail.getByText("pnpm build pnpm test", { exact: true })).toHaveCSS(
        "white-space",
        "pre-wrap",
      );
      await expect(detail.locator("ol > li")).toHaveCount(2);
      await expect(detail.locator("code").filter({ hasText: "Postgres" })).toBeVisible();
      const guidance = detail.getByRole("link", { name: "storage guide", exact: true });
      await expectClarificationReferencePopup(testPage, () => guidance.tap());
      await expect(toggle).toHaveAttribute("aria-expanded", "true");

      const longReference = detail.getByRole("link", { name: LONG_REFERENCE_LABEL, exact: true });
      const bounds = await longReference.boundingBox();
      const detailBounds = await detail.boundingBox();
      expect(bounds).not.toBeNull();
      expect(detailBounds).not.toBeNull();
      expect(bounds!.x).toBeGreaterThanOrEqual(detailBounds!.x - 1);
      expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(
        detailBounds!.x + detailBounds!.width + 1,
      );
      for (const id of ["inbox-history-open-task", "inbox-history-copy-id"]) {
        const action = await row.getByTestId(id).boundingBox();
        expect(action!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
        expect(action!.width).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
      }
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(await testPage.evaluate(() => document.documentElement.clientWidth));
      const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
      const original = messages.find(
        (message) => message.metadata?.pending_id === fixture.pendingId,
      );
      expect(original?.metadata?.status).toBe("pending");
      await prCapture.screenshot("mobile-clarification-markdown-history", {
        caption: "Phone History wraps formatted questions and keeps row actions reachable",
      });
    } finally {
      await apiClient.deleteTask(fixture.taskId);
    }
  });

  test("open-task and copy-id stay reachable with a 44px touch target, with no clipping (AC .14, AC .29)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const title = "Mobile Inbox History Superseded Flow";
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
    });

    const turnAStartedAt = new Date(Date.now() - 120_000).toISOString();
    const turnBStartedAt = new Date(Date.now() - 60_000).toISOString();

    await apiClient.seedSessionMessage(sessionId, {
      type: "clarification_request",
      newTurn: true,
      turnStartedAt: turnAStartedAt,
      turnCompletedAt: turnAStartedAt,
      metadata: {
        pending_id: "pend-mobile-history-superseded-1",
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

    const openTask = row.getByTestId("inbox-history-open-task");
    const copyId = row.getByTestId("inbox-history-copy-id");
    await expect(openTask).toBeVisible();
    await expect(copyId).toBeVisible();

    const copyIdBox = await copyId.boundingBox();
    expect(copyIdBox, "copy-id action should have a box").not.toBeNull();
    expect(copyIdBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(copyIdBox!.width).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    // The document itself must not scroll horizontally on a phone viewport
    // (the row's reason badge, asked time and both actions must compose
    // without clipping).
    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
  });
});
