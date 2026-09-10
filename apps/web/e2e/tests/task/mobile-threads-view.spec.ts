import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { seedSecondaryClarificationTask } from "../../helpers/clarification";
import { createStandardProfile, openTaskSession } from "../../helpers/git-helper";
import { assertNoHorizontalOverflow } from "../../helpers/session-stream-overload";
import { waitForLatestSessionDone } from "../../helpers/session";
import { attachGatewayTrafficCapture, type GatewayTrafficFrame } from "../../helpers/ws-traffic";
import { closeQuickTerminalTab } from "../terminal/terminal-test-helpers";
import { swipeDeckLeft } from "./mobile-threads-swipe-helpers";

const AGENT_TITLE = "Mobile threads live work";

function sentSessionIds(frames: readonly GatewayTrafficFrame[], action: string): string[] {
  return frames
    .filter((frame) => frame.direction === "sent" && frame.action === action && frame.sessionId)
    .map((frame) => frame.sessionId as string);
}

async function expectSwipeCue(page: Page, column: Locator, total: number) {
  const topbar = page.getByTestId("threads-mobile-topbar");
  const cue = topbar.getByTestId("thread-swipe-cue");
  const position = await column.evaluate(
    (element) => Array.from(element.parentElement!.children).indexOf(element) + 1,
  );
  await expect(cue).toHaveText(`${position}/${total}`);
  await expect(page.getByTestId("thread-swipe-cue")).toHaveCount(1);
  await expect(column.getByTestId("thread-swipe-cue")).toHaveCount(0);
  await expect(cue.getByTestId("thread-page-dot")).toHaveCount(total);
  await expect(cue.locator('[data-active="true"]')).toHaveCount(1);
  await expect(cue.getByTestId("thread-page-dot").nth(position - 1)).toHaveAttribute(
    "data-active",
    "true",
  );
  const topbarBox = await topbar.boundingBox();
  const viewBox = await topbar.getByTestId("threads-mobile-view-trigger").boundingBox();
  const menuBox = await topbar.getByTestId("mobile-topbar-menu").boundingBox();
  const cueBox = await cue.boundingBox();
  expect(topbarBox!.height).toBe(56);
  expect(cueBox!.x).toBeGreaterThanOrEqual(viewBox!.x + viewBox!.width);
  expect(cueBox!.x + cueBox!.width).toBeLessThanOrEqual(menuBox!.x);
  expect(cueBox!.y).toBeGreaterThanOrEqual(topbarBox!.y);
  expect(cueBox!.y + cueBox!.height).toBeLessThanOrEqual(topbarBox!.y + topbarBox!.height);
  expect(cueBox!.y + cueBox!.height / 2).toBeCloseTo(viewBox!.y + viewBox!.height / 2, 1);
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
    expect(
      await column
        .locator("header")
        .getByTestId(/^thread-status-/)
        .ariaSnapshot(),
    ).toBe("");

    // The phone layout pages the deck: one column fills the viewport rather
    // than shrinking several into an unreadable row.
    const viewportWidth = testPage.viewportSize()?.width ?? 0;
    const columnWidth = (await column.boundingBox())?.width ?? 0;
    expect(viewportWidth).toBeGreaterThan(0);
    // @covers AC-UI-THREADS-DECK-003.8
    expect(columnWidth).toBeGreaterThanOrEqual(viewportWidth - 2);
    expect(columnWidth).toBeLessThanOrEqual(viewportWidth);
    await expect(testPage.getByTestId("thread-swipe-cue")).toHaveCount(0);

    const viewControl = testPage.getByTestId("threads-mobile-view-trigger");
    await testPage.setViewportSize({ width: 820, height: 1180 });
    await expect(viewControl).toBeVisible();
    await expect(viewControl.getByText("Threads", { exact: true })).toHaveCount(0);
    await expect(column.getByTestId("thread-picker-trigger")).toHaveCount(0);
    expect((await viewControl.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await testPage.setViewportSize({ width: 360, height: 740 });
    await expect(viewControl.getByText("Threads", { exact: true })).toBeVisible();
    await expect(column.getByTestId("thread-picker-trigger")).toBeVisible();
  });

  // @covers AC-UI-THREADS-DECK-003.9, AC-UI-THREADS-DECK-003.10, AC-UI-THREADS-DECK-003.11, AC-UI-THREADS-DECK-003.12
  test("chooses a thread from its title and keeps narrow chat controls contained", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const profile = await createStandardProfile(apiClient, "mobile-threads-picker");
    const titles = [
      "Review checkout accessibility across keyboard and touch",
      "Investigate a very long payment reconciliation identifier",
      "Prepare the release notes",
    ];
    const tasks = [];
    for (const title of titles) {
      const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      await openTaskSession(testPage, title);
      await waitForLatestSessionDone(apiClient, task.id, 1, `agent turn for ${title}`);
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const session = sessions.find((item) => item.is_primary);
      if (!session) throw new Error(`No primary session for ${title}`);
      await apiClient.seedSessionMessage(session.id, {
        type: "message",
        content: `Findings for ${title}.\n\n\`\`\`text\n${"reconciliation_".repeat(30)}\n\`\`\``,
      });
      tasks.push(task);
    }

    await testPage.goto(`/threads?taskId=${tasks[0].id}`);
    await testPage.setViewportSize({ width: 360, height: 740 });
    const board = testPage.getByTestId("threads-board");
    const firstColumn = testPage.getByTestId(`thread-column-${tasks[0].id}`);
    const firstPicker = firstColumn.getByTestId("thread-picker-trigger");
    await expect(firstPicker).toBeVisible();
    await expectSwipeCue(testPage, firstColumn, 3);
    expect((await firstColumn.locator("header").first().boundingBox())!.height).toBeLessThanOrEqual(
      112,
    );
    const titleLines = await firstColumn.getByTestId("thread-mobile-title").evaluate((element) => {
      return (
        element.getBoundingClientRect().height / parseFloat(getComputedStyle(element).lineHeight)
      );
    });
    expect(titleLines).toBeCloseTo(2, 1);
    const viewControl = testPage.getByTestId("threads-mobile-view-trigger");
    const pageLabel = await viewControl.getByText("Threads", { exact: true }).boundingBox();
    const viewLabel = await viewControl.getByText("All threads", { exact: true }).boundingBox();
    expect(viewLabel!.y).toBeGreaterThanOrEqual(pageLabel!.y + pageLabel!.height);
    await firstPicker.tap();
    const picker = testPage.getByRole("dialog", { name: "Choose thread", exact: true });
    await expect(picker).toBeVisible();
    await expect(picker.getByTestId(/^thread-picker-row-/)).toHaveCount(3);
    await picker.getByTestId(`thread-picker-row-${tasks[2].id}`).tap();
    await expect(picker).toBeHidden();

    const selected = testPage.getByTestId(`thread-column-${tasks[2].id}`);
    const selectedPicker = selected.getByTestId("thread-picker-trigger");
    await expect(selectedPicker).toBeFocused();
    await expectSwipeCue(testPage, selected, 3);
    await expect(selected.getByTestId("session-chat")).toBeVisible();
    await expect(board.getByTestId("session-chat")).toHaveCount(1);
    await expect
      .poll(async () => {
        const deckBox = await board.boundingBox();
        const columnBox = await selected.boundingBox();
        return Math.abs((columnBox?.x ?? -100) - (deckBox?.x ?? 0));
      })
      .toBeLessThanOrEqual(1);

    await selectedPicker.tap();
    await expect(picker.getByTestId(`thread-picker-row-${tasks[2].id}`)).toHaveAttribute(
      "aria-current",
      "true",
    );
    await testPage.keyboard.press("Escape");
    await expect(picker).toBeHidden();
    await expect(selectedPicker).toBeFocused();

    await testPage.setViewportSize({ width: 360, height: 740 });
    const viewPicker = testPage.getByTestId("threads-mobile-view-trigger");
    const menu = testPage.getByTestId("mobile-topbar-menu");
    for (const control of [viewPicker, menu, selected.getByTestId("thread-picker-trigger")]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(360);
      expect(
        await control.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          return element.contains(
            document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
          );
        }),
      ).toBe(true);
    }
    await expect(testPage.getByTestId("mobile-topbar-action-strip")).toHaveCount(0);
    const chat = selected.getByTestId("session-chat");
    expect(await chat.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(
      true,
    );
    await assertNoHorizontalOverflow(testPage, "full-width phone Threads");

    await menu.tap();
    await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeVisible();
    await expect(testPage.getByRole("link", { name: "Stats", exact: true })).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeHidden();

    await swipeDeckLeft(testPage);
    await expect(selected.getByTestId("session-chat")).toHaveCount(0);
    await expect(board.getByTestId("session-chat")).toHaveCount(1);
    await expect
      .poll(() =>
        board.evaluate((element) => {
          const page = element.scrollLeft / element.clientWidth;
          return Math.abs(page - Math.round(page));
        }),
      )
      .toBeLessThanOrEqual(0.01);
    const activeColumn = board.getByTestId(/^thread-column-/).filter({
      has: testPage.getByTestId("session-chat"),
    });
    await expectSwipeCue(testPage, activeColumn, 3);
    await testPage.setViewportSize({ width: 820, height: 1180 });
    await expect(testPage.getByTestId("thread-swipe-cue")).toHaveCount(0);
    await testPage.setViewportSize({ width: 360, height: 740 });
    await expect(activeColumn).toHaveCount(1);
    await expectSwipeCue(testPage, activeColumn, 3);
  });

  test("keeps home, status, Quick Chat, and terminal reachable from the menu", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.saveUserSettings({ app_status_bar_enabled: true });
    await testPage.goto("/threads");
    await expect(testPage.getByTestId("thread-swipe-cue")).toHaveCount(0);
    const trigger = testPage.getByTestId("mobile-topbar-menu");
    const menu = testPage.getByRole("dialog", { name: "Menu", exact: true });
    await trigger.tap();
    await expect(menu.getByRole("link", { name: "Home", exact: true })).toBeVisible();
    await expect(menu.getByTestId("mobile-home-status-button")).toBeVisible();
    const quickChat = menu.getByTestId("mobile-quick-chat-button");
    const terminal = menu.getByTestId("mobile-quick-terminal-button");
    for (const action of [quickChat, terminal]) {
      await expect(action).toBeVisible();
      expect((await action.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    await quickChat.tap();
    await expect(menu).toBeHidden();
    const dialog = testPage.getByRole("dialog", { name: "Quick Chat", exact: true });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible();
    await dialog.getByTestId("quick-chat-close").tap();
    await expect(dialog).toBeHidden();
    await trigger.tap();
    await terminal.tap();
    await expect(menu).toBeHidden();
    try {
      await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
    } finally {
      await closeQuickTerminalTab(testPage, dialog.getByTestId("quick-terminal-tab"));
    }
    await expect(dialog.getByTestId("quick-terminal-tab")).toHaveCount(0);
    await expect(dialog).toBeHidden();
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

  test("confirms deletion of a saved view inside the native drawer", async ({
    testPage,
    apiClient,
  }) => {
    const baseView = {
      task_scope: { mode: "all", task_ids: [] },
      filters: [],
      sort: { key: "attention", direction: "asc" },
      max_columns: null,
    };
    const seedResponse = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      thread_views: [
        { ...baseView, id: "view-all-threads", name: "All threads" },
        { ...baseView, id: "view-release", name: "Release threads" },
      ],
      thread_active_view_id: "view-release",
      thread_view_draft: null,
    });
    expect(seedResponse.ok).toBe(true);
    await testPage.goto("/threads");

    const trigger = testPage.getByTestId("threads-mobile-view-trigger");
    await expect(trigger).toContainText("Release threads");
    await trigger.tap();
    const drawer = testPage.getByTestId("threads-mobile-view-drawer");
    await drawer.getByTestId("threads-mobile-view-settings").tap();
    const editor = drawer.getByTestId("threads-view-editor");
    await editor.getByTestId("threads-view-delete").tap();
    const confirmation = editor.getByTestId("saved-task-view-delete-confirmation");
    await expect(confirmation).toHaveAccessibleName("Delete Release threads?");
    await expect(testPage.locator('[role="dialog"]:visible')).toHaveCount(1);
    for (const action of await confirmation.getByRole("button").all()) {
      const box = await action.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    await confirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(drawer).toBeVisible();
    await expect(editor.getByTestId("threads-view-delete")).toBeFocused();

    await editor.getByTestId("threads-view-delete").tap();
    const deletedViewResponse = testPage.waitForResponse(
      (response) =>
        response.ok() &&
        response.request().method() === "PATCH" &&
        response.url().includes("/api/v1/user/settings"),
    );
    await editor
      .getByTestId("saved-task-view-delete-confirmation")
      .getByRole("button", { name: "Delete Release threads" })
      .tap();
    await deletedViewResponse;
    await expect(drawer).toBeHidden();
    await expect(trigger).toContainText("All threads");
    await expect(trigger).toBeFocused();
  });
});
