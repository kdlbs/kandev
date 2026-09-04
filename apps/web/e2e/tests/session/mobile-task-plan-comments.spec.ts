// Filename starts with "mobile-" so this runs under the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

const PLAN_CONTENT = "## Mobile shared plan\n\nReview this mobile implementation step";
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const LEGACY_PRIMARY_ID = "11111111-1111-4111-8111-111111111111";
const LEGACY_SECONDARY_ID = "22222222-2222-4222-8222-222222222222";

test.describe("mobile: task-owned plan comments", () => {
  // @covers AC-TASKS-PLAN-COMMENTS-001.7
  // @covers AC-TASKS-PLAN-COMMENTS-003.2
  // @covers AC-TASKS-PLAN-COMMENTS-004.1
  // @covers AC-TASKS-PLAN-COMMENTS-004.3
  test("migrates shared context and keeps Send and Run routing distinct", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(210_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile task-owned plan comments",
      seedData.agentProfileId,
      {
        description: planScript(PLAN_CONTENT),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected a primary session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
    await session.waitForChatIdle({ timeout: 45_000 });
    await session.openMobileNewSessionDialog();
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().tap();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.filter((candidate) => DONE_STATES.includes(candidate.state)).length;
        },
        { timeout: 60_000 },
      )
      .toBe(2);

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const primary = sessions.find((candidate) => candidate.is_primary);
    const secondary = sessions.find((candidate) => !candidate.is_primary);
    if (!primary || !secondary) throw new Error("expected primary and secondary sessions");

    await testPage.evaluate(
      ({ primaryId, secondaryId, primaryCommentId, secondaryCommentId }) => {
        const base = {
          source: "plan",
          text: "Preserve this migrated mobile feedback",
          selectedText: "Review this mobile implementation step",
          from: 1,
          to: 10,
          createdAt: "2026-09-02T00:00:00Z",
          status: "pending",
        };
        sessionStorage.setItem(
          `kandev.comments.${primaryId}`,
          JSON.stringify([
            { ...base, id: primaryCommentId, sessionId: primaryId },
            {
              id: "legacy-diff",
              sessionId: primaryId,
              source: "diff",
              text: "Keep this local diff feedback",
              filePath: "src/app.ts",
              startLine: 1,
              endLine: 1,
              side: "additions",
              codeContent: "code",
              createdAt: "2026-09-02T00:00:00Z",
              status: "pending",
            },
          ]),
        );
        sessionStorage.setItem(
          `kandev.comments.${secondaryId}`,
          JSON.stringify([{ ...base, id: secondaryCommentId, sessionId: secondaryId }]),
        );
      },
      {
        primaryId: primary.id,
        secondaryId: secondary.id,
        primaryCommentId: LEGACY_PRIMARY_ID,
        secondaryCommentId: LEGACY_SECONDARY_ID,
      },
    );
    await testPage.reload();
    await session.waitForLoad();

    await expect
      .poll(async () => {
        const snapshot = await apiClient.wsRequest<{ comments: Array<{ id: string }> }>(
          "task.plan.comments.list",
          { task_id: task.id },
        );
        return snapshot.comments.map((comment) => comment.id).sort();
      })
      .toEqual([LEGACY_PRIMARY_ID, LEGACY_SECONDARY_ID]);
    await expect(
      testPage.locator('[data-testid="plan-comment-migration-notice"]:visible'),
    ).toHaveCount(0, { timeout: 15_000 });
    await expect(session.activeChat().getByText("2 plan comments", { exact: true })).toBeVisible();
    await expect
      .poll(() =>
        testPage.evaluate(
          (primaryId) => JSON.parse(sessionStorage.getItem(`kandev.comments.${primaryId}`) ?? "[]"),
          primary.id,
        ),
      )
      .toEqual([expect.objectContaining({ id: "legacy-diff", source: "diff" })]);

    const pill = testPage.getByTestId("mobile-sessions-pill");
    await pill.tap();
    await testPage.getByTestId(`mobile-session-row-${secondary.id}`).tap();
    await expect(session.activeChat().getByText("2 plan comments", { exact: true })).toBeVisible();
    await session.sendMessageViaButton("Handle the migrated feedback in this session");
    await expect
      .poll(
        async () =>
          (await apiClient.listSessionMessages(secondary.id)).messages.some(
            (message) =>
              message.author_type === "user" &&
              message.content.includes("migrated mobile feedback") &&
              message.content.includes("Handle the migrated feedback in this session"),
          ),
        { timeout: 30_000 },
      )
      .toBe(true);
    expect(
      (await apiClient.listSessionMessages(primary.id)).messages.some((message) =>
        message.content.includes("migrated mobile feedback"),
      ),
    ).toBe(false);
    await session.waitForChatIdle({ timeout: 45_000 });
    // A completed seed turn can auto-promote the secondary. Pin the intended
    // primary so this flow isolates selected-session Send from primary Run.
    await apiClient.setPrimarySession(primary.id);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).primary_session_id, {
        timeout: 15_000,
      })
      .toBe(primary.id);
    await pill.tap();
    await expect(
      testPage.getByTestId(`mobile-session-row-${primary.id}`).locator(".tabler-icon-star"),
    ).toBeVisible({ timeout: 15_000 });
    await testPage.getByTestId(`mobile-session-row-${secondary.id}`).tap();

    await session.togglePlanMode();
    await testPage.getByRole("button", { name: "Plan", exact: true }).tap();
    const editor = session.planPanel.locator(".ProseMirror:visible");
    await expect(editor).toBeVisible({ timeout: 10_000 });
    await editor.focus();
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+a`);
    const commentAction = testPage.getByTestId("plan-formatting-action-comment");
    await expect(commentAction).toBeVisible();
    await expect(commentAction).toBeEnabled();
    await commentAction.tap();

    const drawer = testPage.getByTestId("plan-comment-drawer");
    await expect(drawer).toBeVisible({ timeout: 5_000 });
    const runComment = "Run this mobile feedback in primary";
    await drawer.locator("textarea").fill(runComment);
    const actionButtons = drawer.getByRole("button");
    const sizes = await actionButtons.evaluateAll((buttons) =>
      buttons.map((button) => button.getBoundingClientRect().height),
    );
    for (const height of sizes) expect(height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile plan comment drawer");
    const run = drawer.getByRole("button", { name: "Run", exact: true });
    await expect(run).toBeEnabled();
    await run.tap();

    await expect
      .poll(
        async () =>
          (await apiClient.listSessionMessages(primary.id)).messages.some(
            (message) => message.author_type === "user" && message.content.includes(runComment),
          ),
        { timeout: 30_000 },
      )
      .toBe(true);
    expect(
      (await apiClient.listSessionMessages(secondary.id)).messages.some((message) =>
        message.content.includes(runComment),
      ),
    ).toBe(false);
  });
});
