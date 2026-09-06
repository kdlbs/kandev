import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { seedSecondaryClarificationTask } from "../../helpers/clarification";
import { createStandardProfile, openTaskSession } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";
import { attachGatewayTrafficCapture, type GatewayTrafficFrame } from "../../helpers/ws-traffic";

const AGENT_TITLE = "Threads live agent work";
const SECOND_TITLE = "Threads second live agent work";
const IDLE_TITLE = "Threads never started";

function sentSessionIds(frames: readonly GatewayTrafficFrame[], action: string): string[] {
  return frames
    .filter((frame) => frame.direction === "sent" && frame.action === action && frame.sessionId)
    .map((frame) => frame.sessionId as string);
}

function activeSessionIds(frames: readonly GatewayTrafficFrame[]): string[] {
  const active = new Set<string>();
  for (const frame of frames) {
    if (frame.direction !== "sent" || !frame.sessionId) continue;
    if (frame.action === "session.subscribe") active.add(frame.sessionId);
    if (frame.action === "session.unsubscribe") active.delete(frame.sessionId);
  }
  return [...active];
}

function columnOrder(board: Locator): Promise<string[]> {
  return board
    .locator("[data-thread-column-id]")
    .evaluateAll((columns) =>
      columns.map((column) => column.getAttribute("data-thread-column-id") ?? ""),
    );
}

async function visibleColumnTaskIds(board: Locator): Promise<string[]> {
  return board.evaluate((node) => {
    const boardRect = node.getBoundingClientRect();
    return [...node.querySelectorAll<HTMLElement>("[data-thread-column-id]")]
      .filter((column) => {
        const rect = column.getBoundingClientRect();
        return rect.right > boardRect.left && rect.left < boardRect.right;
      })
      .map((column) => column.dataset.threadColumnId ?? "");
  });
}

/**
 * Runs a real agent turn so the task ends with a settled primary session.
 * Seeded sessions are never marked primary, and the deck follows production's
 * primary-session rule, so they would not appear.
 *
 * The task page is what launches the session (`useEnsureTaskSession`), so
 * opening it is part of arranging the fixture, not an assertion.
 */
async function startAgentTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  profileName: string,
  options: { title?: string } = {},
) {
  const title = options.title ?? AGENT_TITLE;
  const profile = await createStandardProfile(apiClient, profileName);
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  await openTaskSession(page, title);
  // `POST /tasks` answers with the Task itself, so the id lives on `id` — the
  // `task_id` shape belongs to the seed harness.
  await waitForLatestSessionDone(apiClient, task.id, 1, `agent turn for ${title}`);
  return task;
}

test.describe("Threads view", () => {
  test("decks live agent threads and leaves work with no agent out", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const live = await startAgentTask(testPage, apiClient, seedData, "threads-live");
    const idle = await apiClient.seedTask(seedData.workspaceId, IDLE_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/");
    await expect(new KanbanPage(testPage).board).toBeVisible();
    await testPage.getByTestId("view-toggle-threads").click();
    await expect(testPage).toHaveURL(/\/threads/);

    await expect(testPage.getByTestId("threads-board")).toBeVisible();
    const column = testPage.getByTestId(`thread-column-${live.id}`);
    await expect(column).toBeVisible();
    await expect(column).toContainText(AGENT_TITLE);
    // The simple workflow moves the completed task into its review step.
    await expect(column.getByTestId("thread-status-review-ready")).toBeVisible();
    await expect(testPage.getByTestId(`thread-column-${idle.task_id}`)).toHaveCount(0);
  });

  test("remembers Threads as the device listing view and reopens Home there", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const live = await startAgentTask(testPage, apiClient, seedData, "threads-remembered");

    await testPage.goto("/");
    await expect(new KanbanPage(testPage).board).toBeVisible();
    await testPage.getByTestId("view-toggle-threads").click();
    await expect(testPage).toHaveURL(/\/threads/);
    await expect(testPage.getByTestId("view-toggle-threads")).toHaveAttribute("data-state", "on");

    await testPage.goto("/");
    await expect(testPage).toHaveURL(/\/threads/);
    await expect(testPage.getByTestId(`thread-column-${live.id}`)).toBeVisible();
  });

  test("opens the full task page from a column", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(180_000);
    const live = await startAgentTask(testPage, apiClient, seedData, "threads-open-task");

    await testPage.goto("/threads");
    const column = testPage.getByTestId(`thread-column-${live.id}`);
    await expect(column).toBeVisible();

    await column.getByRole("button", { name: "Open task" }).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${live.id}`));
  });

  test("hands a discussion back to the deck, scrolled to its column", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const live = await startAgentTask(testPage, apiClient, seedData, "threads-round-trip");

    // startAgentTask leaves the browser on the task page, which is the surface
    // the button lives on.
    await testPage.getByTestId("open-in-threads-button").click();

    await expect(testPage).toHaveURL(new RegExp(`/threads\\?taskId=${live.id}`));
    const column = testPage.getByTestId(`thread-column-${live.id}`);
    await expect(column).toBeVisible();
    await expect(column).toHaveAttribute("data-focused", "true");
    // The deck is the round trip's destination, so it must not re-offer the jump.
    await expect(testPage.getByTestId("open-in-threads-button")).toHaveCount(0);

    // The mark answers "where is it"; once the reader starts working it has
    // nothing left to say and must not sit on the column indefinitely.
    await column.locator(".tiptap.ProseMirror").click();
    await expect(column).not.toHaveAttribute("data-focused", "true");
  });

  test("shares the board width between columns instead of leaving it empty", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const first = await startAgentTask(testPage, apiClient, seedData, "threads-width-a");
    const second = await startAgentTask(testPage, apiClient, seedData, "threads-width-b", {
      title: SECOND_TITLE,
    });

    await testPage.goto("/threads");
    const board = testPage.getByTestId("threads-board");
    await expect(board).toBeVisible();
    const columns = [
      testPage.getByTestId(`thread-column-${first.id}`),
      testPage.getByTestId(`thread-column-${second.id}`),
    ];
    for (const column of columns) await expect(column).toBeVisible();

    const boardWidth = (await board.boundingBox())?.width ?? 0;
    const widths = await Promise.all(
      columns.map(async (column) => (await column.boundingBox())?.width ?? 0),
    );
    expect(boardWidth).toBeGreaterThan(0);
    // Two threads on a desktop board fill it rather than sitting at a fixed
    // 380px with the rest of the deck blank.
    expect(Math.min(...widths)).toBeGreaterThan(400);
    expect(widths[0] + widths[1]).toBeGreaterThan(boardWidth * 0.8);
  });

  test("saves a capped view, switches views, and restores it after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(360_000);
    await startAgentTask(testPage, apiClient, seedData, "threads-saved-view-a");
    await startAgentTask(testPage, apiClient, seedData, "threads-saved-view-b", {
      title: "Threads saved view second work",
    });
    await startAgentTask(testPage, apiClient, seedData, "threads-saved-view-c", {
      title: "Threads saved view third work",
    });

    await testPage.goto("/threads");
    const board = testPage.getByTestId("threads-board");
    await expect(board.locator("[data-thread-column-id]")).toHaveCount(3);

    await testPage.getByTestId("threads-view-picker").click();
    await testPage.getByTestId("threads-new-view").click();
    const editor = testPage.getByTestId("threads-view-settings-popover");
    await expect(editor).toBeVisible();
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
    await editor.getByTestId("threads-view-save").click();
    await savedViewResponse;

    await expect(board.locator("[data-thread-column-id]")).toHaveCount(1);
    await expect(testPage.getByTestId("threads-view-count")).toContainText("1 of 3 columns");
    await expect(testPage.getByTestId("threads-view-count")).toContainText("2 hidden");

    // The saved view remains active after a full page bootstrap.
    await testPage.reload();
    await expect(board.locator("[data-thread-column-id]")).toHaveCount(1);
    await expect(testPage.getByTestId("threads-view-picker")).toContainText("New view");

    // Switching back to the built-in view restores all eligible columns. The
    // sidebar's saved-view state is not involved in this interaction.
    await testPage.getByTestId("threads-view-picker").click();
    await testPage.getByTestId("threads-view-option-view-all-threads").click();
    await expect(board.locator("[data-thread-column-id]")).toHaveCount(3);
  });

  test("explains sorts and shows live task details in the task picker", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const task = await startAgentTask(testPage, apiClient, seedData, "threads-view-task-details", {
      title: "Threads view task details",
    });
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      task_id: task.id,
      owner: "kandev-e2e",
      repo: "threads-view-details",
      pr_number: 42,
      pr_url: "https://github.test/kandev-e2e/threads-view-details/pull/42",
      pr_title: "Show Threads view task details",
      head_branch: "feature/threads-view-details",
      base_branch: "main",
      author_login: "threads-author",
      state: "open",
      review_state: "approved",
      checks_state: "failure",
      mergeable_state: "blocked",
    });

    await testPage.goto("/threads");
    await testPage.getByTestId("threads-view-picker").click();
    await testPage.getByTestId("threads-new-view").click();
    const editor = testPage.getByTestId("threads-view-settings-popover");
    await expect(editor).toBeVisible();
    await expect(editor).toHaveCSS("border-top-width", "1px");
    await expect(editor.getByTestId("threads-max-columns")).toHaveValue("5");

    await editor.getByTestId("threads-sort-select").click();
    const attention = testPage.getByRole("option", { name: "Attention" });
    await expect(attention).toContainText("need you");
    await attention.click();

    await editor.getByTestId("threads-scope-select").click();
    await testPage.getByRole("option", { name: "Selected tasks", exact: true }).click();
    await editor.getByTestId("threads-open-task-picker").click();
    const row = editor.getByTestId("threads-task-picker-row").filter({
      hasText: "Threads view task details",
    });
    await expect(row.getByRole("img", { name: "Review" })).toBeVisible();
    await expect(row.getByTestId("threads-task-picker-step")).toHaveText("Review");

    await apiClient.updateTaskState(task.id, "WAITING_FOR_INPUT");
    await expect(row.getByTestId("task-state-waiting-for-input")).toBeVisible();

    const prIcon = row.getByTestId(`pr-task-icon-${task.id}`);
    await expect(prIcon).toBeVisible();
    await expect(prIcon).toHaveClass(/text-/);
    await prIcon.hover();
    await expect(
      testPage
        .locator('[data-slot="tooltip-content"]:visible')
        .filter({ has: testPage.getByTestId("pr-task-status-summary") })
        .last(),
    ).toBeVisible();
  });

  test("shows which column the cursor is in, and moves that mark on click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const first = await startAgentTask(testPage, apiClient, seedData, "threads-focus-a");
    const second = await startAgentTask(testPage, apiClient, seedData, "threads-focus-b", {
      title: SECOND_TITLE,
    });

    await testPage.goto("/threads");
    const columns = {
      first: testPage.getByTestId(`thread-column-${first.id}`),
      second: testPage.getByTestId(`thread-column-${second.id}`),
    };
    for (const column of Object.values(columns)) await expect(column).toBeVisible();

    // The composer's own border tracks agent state, not the caret, so the
    // column has to carry the focus mark or a deck of composers gives the
    // reader no way to tell where typing will land.
    const ringed = (column: Locator) =>
      column.evaluate((node) => getComputedStyle(node).boxShadow !== "none");

    await columns.second.locator(".tiptap.ProseMirror").click();
    await expect.poll(() => ringed(columns.second)).toBe(true);
    expect(await ringed(columns.first)).toBe(false);

    await columns.first.locator(".tiptap.ProseMirror").click();
    await expect.poll(() => ringed(columns.first)).toBe(true);
    expect(await ringed(columns.second)).toBe(false);
  });

  test("holds a column's slot and the deck's scroll while you reply to it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    await startAgentTask(testPage, apiClient, seedData, "threads-order-a");
    await startAgentTask(testPage, apiClient, seedData, "threads-order-b", {
      title: SECOND_TITLE,
    });

    await testPage.goto("/threads");
    const board = testPage.getByTestId("threads-board");
    await expect(board).toBeVisible();
    const columnIds = () =>
      board.evaluate((node) =>
        [...node.querySelectorAll('[data-testid^="thread-column-"]')].map(
          (column) => column.getAttribute("data-testid") ?? "",
        ),
      );

    const orderBefore = await columnIds();
    expect(orderBefore).toHaveLength(2);
    const scrollBefore = await board.evaluate((node) => node.scrollLeft);

    // Reply to the column ranked LAST. Ranking puts the most recent activity
    // first, so this is the thread whose slot a live re-rank would visibly
    // move, right while the reader is typing into it.
    const targetTaskId = orderBefore[orderBefore.length - 1].replace("thread-column-", "");
    const target = testPage.getByTestId(`thread-column-${targetTaskId}`);
    const editor = target.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.fill("/e2e:simple-message");
    await editor.press("Control+Enter");

    // A whole turn's worth of task updates reaches the deck before this
    // resolves, which is more than enough re-ranking pressure to move a column.
    await waitForLatestSessionDone(apiClient, targetTaskId, 1, "reply turn never settled");
    await expect(target.getByTestId("thread-status-review-ready")).toBeVisible({
      timeout: 30_000,
    });

    expect(await columnIds()).toEqual(orderBefore);
    expect(await board.evaluate((node) => node.scrollLeft)).toBe(scrollBefore);
  });

  test("explains an empty deck when nothing is running", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.seedTask(seedData.workspaceId, IDLE_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/threads");
    await expect(testPage.getByTestId("threads-empty-state")).toBeVisible();
    await expect(testPage.getByTestId("threads-board")).toHaveCount(0);
  });

  test("switches sessions and bounds desktop detail streams to the viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(300_000);
    const target = await seedSecondaryClarificationTask(
      apiClient,
      seedData,
      "Threads multi-session target",
    );
    const first = await startAgentTask(testPage, apiClient, seedData, "threads-window-first", {
      title: "Threads window first",
    });
    const second = await startAgentTask(testPage, apiClient, seedData, "threads-window-second", {
      title: "Threads window second",
    });
    const third = await startAgentTask(testPage, apiClient, seedData, "threads-window-third", {
      title: "Threads window third",
    });

    const sessionByTask = new Map([
      [target.id, target.primarySessionId],
      [first.id, first.session_id],
      [second.id, second.session_id],
      [third.id, third.session_id],
    ]);
    const capture = attachGatewayTrafficCapture(testPage);
    await testPage.goto(`/threads?taskId=${target.id}&sessionId=${target.clarificationSessionId}`);

    const board = testPage.getByTestId("threads-board");
    await expect(board).toBeVisible();
    await expect(board.locator("[data-thread-column-id]")).toHaveCount(4);
    const targetColumn = testPage.getByTestId(`thread-column-${target.id}`);
    const primaryTab = targetColumn.getByTestId(`thread-session-tab-${target.primarySessionId}`);
    const siblingTab = targetColumn.getByTestId(
      `thread-session-tab-${target.clarificationSessionId}`,
    );
    await expect(primaryTab).toBeVisible();
    await expect(siblingTab).toBeVisible();
    await expect(siblingTab).toHaveAttribute("data-state", "active");
    await siblingTab
      .locator("span")
      .last()
      .evaluate((node) => {
        node.textContent =
          "A session label that is intentionally much longer than the compact metadata row";
      });
    const desktopTabGeometry = await targetColumn.evaluate((column) => {
      const switcher = column.querySelector<HTMLElement>('[data-testid="thread-session-switcher"]');
      return {
        columnOverflow: column.scrollWidth > column.clientWidth,
        switcherOverflow: switcher ? switcher.scrollWidth > switcher.clientWidth : true,
        documentOverflow:
          document.documentElement.scrollWidth > document.documentElement.clientWidth,
      };
    });
    expect(desktopTabGeometry).toEqual({
      columnOverflow: false,
      switcherOverflow: false,
      documentOverflow: false,
    });
    await expect(testPage).toHaveURL(
      new RegExp(`taskId=${target.id}.*sessionId=${target.clarificationSessionId}`),
    );

    await expect
      .poll(() => sentSessionIds(capture.frames, "session.subscribe"), {
        timeout: 30_000,
        message: "Threads did not subscribe the deep-linked sibling",
      })
      .toContain(target.clarificationSessionId);

    const initialVisibleTaskIds = await visibleColumnTaskIds(board);
    const initialExpectedSessionIds = initialVisibleTaskIds
      .map((taskId) =>
        taskId === target.id ? target.clarificationSessionId : sessionByTask.get(taskId),
      )
      .filter((sessionId): sessionId is string => Boolean(sessionId));
    await expect
      .poll(
        () => {
          const subscribed = new Set(sentSessionIds(capture.frames, "session.subscribe"));
          return initialExpectedSessionIds.every((sessionId) => subscribed.has(sessionId));
        },
        { timeout: 30_000, message: "visible task columns did not activate their sessions" },
      )
      .toBe(true);

    const initialSubscribed = new Set(activeSessionIds(capture.frames));
    expect(initialSubscribed).toEqual(new Set(initialExpectedSessionIds));
    expect(initialSubscribed).not.toContain(target.primarySessionId);
    expect(
      sentSessionIds(capture.frames, "message.list"),
      "the unselected sibling must not request a transcript",
    ).not.toContain(target.primarySessionId);
    await expect(targetColumn.getByRole("button", { name: /add|new/i })).toHaveCount(0);

    // The session linked from the task page is a settled clarification sibling.
    // Switching back to the primary replaces only this column's detail stream;
    // the sibling remains visible as an exact inactive status item.
    await primaryTab.click();
    await expect(primaryTab).toHaveAttribute("data-state", "active");
    await expect(siblingTab.locator('[aria-label="Question from agent"]')).toBeVisible();
    await expect
      .poll(() => sentSessionIds(capture.frames, "session.subscribe"), {
        timeout: 30_000,
        message: "switching tabs did not subscribe the primary session",
      })
      .toContain(target.primarySessionId);
    await expect
      .poll(() => sentSessionIds(capture.frames, "session.unsubscribe"), {
        timeout: 30_000,
        message: "switching tabs did not release the sibling session",
      })
      .toContain(target.clarificationSessionId);

    const reviewTask = first.id;
    await apiClient.updateTaskState(reviewTask, "REVIEW");
    await expect
      .poll(() => apiClient.getTask(reviewTask).then((task) => task.state))
      .toBe("REVIEW");
    await expect(testPage.getByTestId(`thread-column-${reviewTask}`)).toContainText(
      "Ready for review",
    );
    await expect(
      testPage.getByTestId(`thread-column-${reviewTask}`).getByTestId("thread-status-review-ready"),
    ).toBeVisible();

    const orderBeforeScroll = await columnOrder(board);
    const visibleBeforeScroll = await visibleColumnTaskIds(board);
    const allTaskIds = [target.id, first.id, second.id, third.id];
    const scrollTargetTaskId =
      allTaskIds.find((taskId) => !visibleBeforeScroll.includes(taskId)) ?? allTaskIds.at(-1);
    if (!scrollTargetTaskId) throw new Error("Threads did not produce a scroll target");
    const scrollTargetSessionId =
      scrollTargetTaskId === target.id
        ? target.primarySessionId
        : sessionByTask.get(scrollTargetTaskId);
    if (!scrollTargetSessionId) throw new Error("Threads scroll target has no session");
    const scrollBefore = await board.evaluate((node) => node.scrollLeft);
    const scrollTarget = testPage.getByTestId(`thread-column-${scrollTargetTaskId}`);
    await scrollTarget.evaluate((column) =>
      column.scrollIntoView({ inline: "end", block: "nearest" }),
    );
    await expect
      .poll(() => board.evaluate((node) => node.scrollLeft), {
        timeout: 15_000,
        message: "horizontal scroll did not move the Threads board",
      })
      .not.toBe(scrollBefore);
    await expect
      .poll(() => sentSessionIds(capture.frames, "session.subscribe"), {
        timeout: 30_000,
        message: "scrolling did not activate the newly visible session",
      })
      .toContain(scrollTargetSessionId);
    const visibleAfterScroll = await visibleColumnTaskIds(board);
    const departedSessionIds = visibleBeforeScroll
      .filter((taskId) => !visibleAfterScroll.includes(taskId))
      .map((taskId) => (taskId === target.id ? target.primarySessionId : sessionByTask.get(taskId)))
      .filter((sessionId): sessionId is string => Boolean(sessionId));
    await expect
      .poll(
        () =>
          departedSessionIds.some((sessionId) =>
            sentSessionIds(capture.frames, "session.unsubscribe").includes(sessionId),
          ),
        {
          timeout: 30_000,
          message: "scrolling did not release the departed detail",
        },
      )
      .toBe(true);
    expect(await columnOrder(board)).toEqual(orderBeforeScroll);

    await test.info().attach("threads-desktop-subscription-evidence.json", {
      body: JSON.stringify(
        {
          initialVisibleTaskIds,
          initialExpectedSessionIds,
          initialSubscribed: [...initialSubscribed],
          subscribeSessionIds: sentSessionIds(capture.frames, "session.subscribe"),
          unsubscribeSessionIds: sentSessionIds(capture.frames, "session.unsubscribe"),
          visibleBeforeScroll,
          visibleAfterScroll,
          scrollTargetTaskId,
          columnOrder: orderBeforeScroll,
        },
        null,
        2,
      ),
      contentType: "application/json",
    });
  });
});
