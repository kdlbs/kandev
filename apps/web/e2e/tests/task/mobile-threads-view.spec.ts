import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { seedSecondaryClarificationTask } from "../../helpers/clarification";
import { createStandardProfile, openTaskSession } from "../../helpers/git-helper";
import { assertNoHorizontalOverflow } from "../../helpers/session-stream-overload";
import { waitForLatestSessionDone } from "../../helpers/session";
import { attachGatewayTrafficCapture, type GatewayTrafficFrame } from "../../helpers/ws-traffic";

const AGENT_TITLE = "Mobile threads live work";

function sentSessionIds(frames: readonly GatewayTrafficFrame[], action: string): string[] {
  return frames
    .filter((frame) => frame.direction === "sent" && frame.action === action && frame.sessionId)
    .map((frame) => frame.sessionId as string);
}

test.describe("Mobile Threads view", () => {
  test("reaches the deck from the drawer and pages one full-width column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const profile = await createStandardProfile(apiClient, "mobile-threads");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      AGENT_TITLE,
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await openTaskSession(testPage, AGENT_TITLE);
    await waitForLatestSessionDone(apiClient, task.id, 1, `agent turn for ${AGENT_TITLE}`);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileMenuButton.click();
    const menu = testPage.getByRole("dialog", { name: "Menu" });
    await menu.getByRole("radio", { name: "Threads", exact: true }).click();
    await expect(testPage).toHaveURL(/\/threads/);

    const column = testPage.getByTestId(`thread-column-${task.id}`);
    await expect(column).toBeVisible();
    await expect(column).toContainText(AGENT_TITLE);

    // The phone layout pages the deck: one column fills the viewport rather
    // than shrinking several into an unreadable row.
    const viewportWidth = testPage.viewportSize()?.width ?? 0;
    const columnWidth = (await column.boundingBox())?.width ?? 0;
    expect(viewportWidth).toBeGreaterThan(0);
    expect(columnWidth).toBeGreaterThan(viewportWidth * 0.7);
    expect(columnWidth).toBeLessThanOrEqual(viewportWidth);
  });

  test("uses a bounded native picker for the selected session on phone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const target = await seedSecondaryClarificationTask(
      apiClient,
      seedData,
      "Mobile threads multi-session target",
    );
    const capture = attachGatewayTrafficCapture(testPage);
    await testPage.goto(`/threads?taskId=${target.id}&sessionId=${target.clarificationSessionId}`);

    const board = testPage.getByTestId("threads-board");
    const column = testPage.getByTestId(`thread-column-${target.id}`);
    await expect(board).toBeVisible();
    await expect(column).toBeVisible();
    const picker = column.getByTestId("thread-session-picker-trigger");
    await expect(picker).toBeVisible();
    await expect
      .poll(() => sentSessionIds(capture.frames, "session.subscribe"), {
        timeout: 30_000,
        message: "mobile Threads did not subscribe the deep-linked session",
      })
      .toContain(target.clarificationSessionId);
    expect(sentSessionIds(capture.frames, "session.subscribe")).not.toContain(
      target.primarySessionId,
    );
    await expect(column.getByTestId("session-chat")).toHaveCount(1);

    await picker.tap();
    const sheet = testPage.getByRole("dialog", { name: "Select session" });
    await expect(sheet).toBeVisible();
    const sheetContent = testPage.getByTestId("thread-session-picker-sheet");
    await expect(sheetContent).toBeVisible();
    await expect
      .poll(() => sheet.getByTestId(/^thread-session-row-/).count(), {
        timeout: 15_000,
        message: "mobile session picker did not load every task session",
      })
      .toBe(2);
    await expect(
      sheet.getByTestId(`thread-session-row-${target.clarificationSessionId}`),
    ).toHaveAttribute("aria-current", "true");
    const primaryRow = sheet.getByTestId(`thread-session-row-${target.primarySessionId}`);
    const primaryRowBox = await primaryRow.boundingBox();
    expect(primaryRowBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(await sheetContent.evaluate((element) => element.className)).toContain(
      "safe-area-inset-bottom",
    );

    await primaryRow.tap();
    await expect(sheet).toBeHidden();
    await expect
      .poll(() => sentSessionIds(capture.frames, "session.subscribe"), {
        timeout: 30_000,
        message: "mobile picker did not activate the primary session",
      })
      .toContain(target.primarySessionId);
    await expect
      .poll(() => sentSessionIds(capture.frames, "session.unsubscribe"), {
        timeout: 30_000,
        message: "mobile picker did not release the sibling session",
      })
      .toContain(target.clarificationSessionId);
    await expect(column.getByTestId("session-chat")).toHaveCount(1);
    await assertNoHorizontalOverflow(testPage, "mobile Threads session picker");
  });

  test("switches and edits saved views in one native drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const profile = await createStandardProfile(apiClient, "mobile-threads-saved-view");
    const firstTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile saved view first work",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const secondTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile saved view second work",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await openTaskSession(testPage, "Mobile saved view first work");
    await waitForLatestSessionDone(apiClient, firstTask.id, 1, "first saved view agent turn");
    await openTaskSession(testPage, "Mobile saved view second work");
    await waitForLatestSessionDone(apiClient, secondTask.id, 1, "second saved view agent turn");

    await testPage.goto("/threads");
    const trigger = testPage.getByTestId("threads-mobile-view-trigger");
    await expect(trigger).toBeVisible();
    await trigger.tap();

    const drawer = testPage.getByTestId("threads-mobile-view-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("threads-mobile-view-list")).toBeVisible();
    await drawer.getByTestId("threads-mobile-new-view").tap();

    const editor = drawer.getByTestId("threads-view-editor");
    await expect(editor).toBeVisible();
    await expect(editor.getByTestId("threads-max-columns")).toHaveValue("5");
    await editor.getByTestId("threads-scope-select").tap();
    await testPage.getByRole("option", { name: "Selected tasks", exact: true }).tap();
    await editor.getByTestId("threads-open-task-picker").tap();
    await expect(drawer.getByTestId("threads-task-picker")).toBeVisible();
    await drawer.getByTestId("threads-task-picker-search").fill("Mobile saved");
    await expect(drawer.getByTestId("threads-task-picker-row")).toHaveCount(2);
    await expect(
      drawer.getByTestId("threads-task-picker-row").first().getByRole("img", { name: "Review" }),
    ).toBeVisible();
    await expect(
      drawer.getByTestId("threads-task-picker-row").first().getByTestId("threads-task-picker-step"),
    ).toHaveText("Review");
    await drawer.getByTestId("threads-task-picker-select-all").tap();
    const row = drawer.getByTestId("threads-task-picker-row").first();
    const rowBox = await row.boundingBox();
    expect(rowBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    await drawer.getByTestId("threads-task-picker-back").tap();
    await expect(editor).toBeVisible();

    await editor.getByTestId("threads-filter-add").tap();
    await editor.getByTestId("threads-filter-dimension").tap();
    await testPage.getByRole("option", { name: "Title", exact: true }).tap();
    await editor.getByTestId("threads-filter-value").fill("Mobile saved view");
    await editor.getByTestId("threads-sort-select").tap();
    const titleSort = testPage.getByRole("option", { name: "Title", exact: true });
    await expect(titleSort).toContainText("alphabetical");
    await titleSort.tap();
    await editor.getByTestId("threads-max-columns").fill("1");

    const savedViewResponse = testPage.waitForResponse((response) => {
      const request = response.request();
      if (
        !response.ok() ||
        request.method() !== "PATCH" ||
        !request.url().includes("/api/v1/user/settings")
      ) {
        return false;
      }
      const payload = request.postDataJSON() as {
        thread_view_draft?: unknown;
        thread_views?: Array<{ max_columns?: number | null }>;
      } | null;
      return (
        payload?.thread_view_draft === null &&
        payload.thread_views?.some((view) => view.max_columns === 1) === true
      );
    });
    await editor.getByTestId("threads-view-save").tap();
    await savedViewResponse;

    await drawer.getByTestId("threads-mobile-view-back").tap();
    await drawer
      .locator('[data-testid^="threads-mobile-view-option-"]')
      .filter({ hasText: "New view" })
      .tap();
    const board = testPage.getByTestId("threads-board");
    await expect(board.locator("[data-thread-column-id]")).toHaveCount(1);

    await testPage.reload();
    await expect(testPage.getByTestId("threads-mobile-view-trigger")).toContainText("New view");
    await expect(
      testPage.getByTestId("threads-board").locator("[data-thread-column-id]"),
    ).toHaveCount(1);

    const reloadedTrigger = testPage.getByTestId("threads-mobile-view-trigger");
    await reloadedTrigger.tap();
    const reloadedDrawer = testPage.getByTestId("threads-mobile-view-drawer");
    await reloadedDrawer.getByTestId("threads-mobile-view-option-view-all-threads").tap();
    await expect(
      testPage.getByTestId("threads-board").locator("[data-thread-column-id]"),
    ).toHaveCount(2);

    const drawerAfterSwitch = testPage.getByTestId("threads-mobile-view-drawer");
    await expect(drawerAfterSwitch).toBeHidden();
    await expect(reloadedTrigger).toBeFocused();

    // The settings list remains a native touch surface after the saved view
    // round trip. Reopen it to validate the same geometry and safe-area
    // contract used by the editor and picker pages.
    await reloadedTrigger.tap();
    const geometryDrawer = testPage.getByTestId("threads-mobile-view-drawer");
    await expect(geometryDrawer).toBeVisible();
    await expect(geometryDrawer.getByTestId("threads-mobile-view-list")).toBeVisible();
    const buttons = geometryDrawer.locator("button:visible");
    const buttonCount = await buttons.count();
    for (let index = 0; index < buttonCount; index += 1) {
      const box = await buttons.nth(index).boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    await assertNoHorizontalOverflow(testPage, "mobile Threads saved views");
    await expect(geometryDrawer).toHaveClass(/safe-area-inset-bottom/);

    await geometryDrawer.getByTestId("threads-mobile-view-option-view-all-threads").tap();
    await expect(geometryDrawer).toBeHidden();
    await expect(reloadedTrigger).toBeFocused();
  });
});
