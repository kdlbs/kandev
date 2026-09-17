import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  MIDDLE_PROMPT_MARKER,
  seedLongPromptHistory,
} from "../../helpers/prompt-history-long-seed";
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const SENTINEL = "sentinel-jump-token-9f3a";

// `formatPromptDuration` shapes: 0s / 5s / 5m 23s / 1h 2m 3s. Elapsed wall
// time between seeding and sending is uncontrolled, so only the shape is
// asserted (exact-`0s` stays in fixed-time unit coverage).
const DURATION_SHAPE = /^\d+s$|^\d+m \d+s$|^\d+h \d+m \d+s$/;

async function revealPromptHistoryTarget(panel: Locator, targetRow: Locator) {
  const scroller = panel.getByTestId("prompt-history-scroll");
  await expect(scroller).toBeVisible();
  for (let attempt = 0; attempt < 10; attempt++) {
    if ((await targetRow.count()) > 0) return;
    await scroller.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await targetRow
      .first()
      .waitFor({ state: "attached", timeout: 5_000 })
      .catch(() => undefined);
  }
  if ((await targetRow.count()) === 0) {
    throw new Error("prompt history target did not load after 10 scroll attempts");
  }
}

test.describe("Prompt history panel", () => {
  test("lists prompts newest-first with durations, caps expansion, and jumps to the transcript", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const seedPrompt = "Prompt history seeded user prompt";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Prompt history task",
      seedData.agentProfileId,
      {
        description: seedPrompt,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Prompt history task did not create a session");
    const sessionId = task.session_id;
    /** Polls the task's sessions until one has reached a terminal DONE_STATES state. */
    const settled = async () => {
      const { sessions } = await apiClient.listTaskSessions(task.id);
      return DONE_STATES.includes(sessions[0]?.state ?? "");
    };
    await expect
      .poll(settled, {
        timeout: 45_000,
        message: "Waiting for the seeded prompt-history session to settle",
      })
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();

    // (1) Open Prompt history via the "+" menu and measure the panel root.
    await session.addPanelButton().click();
    await testPage.getByTestId("add-panel-prompt-history-item").click();
    const panel = testPage.getByTestId("prompt-history-panel");
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("prompt-history-row-0")).toContainText(seedPrompt);

    const metrics = await panel.evaluate((el) => {
      const row = el.querySelector('[data-testid^="prompt-history-row-"]');
      return {
        clientWidth: el.clientWidth,
        clientHeight: el.clientHeight,
        rowHeight: row?.getBoundingClientRect().height ?? 24,
      };
    });
    // Payload at >=2x the lines needed to fill 40% of the panel height. A
    // space-separated sentence (repeated words) so it wraps and CANNOT stay
    // on one line; the sentinel token stays unique to this prompt.
    const charsPerLine = Math.max(1, Math.floor(metrics.clientWidth / 8));
    const capPx = metrics.clientHeight * 0.4;
    const requiredLines = Math.max(1, Math.ceil(capPx / metrics.rowHeight));
    const payloadLines = Math.max(requiredLines * 2, 8);
    const longPrompt = `${Array.from({ length: Math.ceil((charsPerLine * payloadLines) / 5) }, () => "word").join(" ")} ${SENTINEL}`;

    // (2) Activate the chat tab and send the second prompt through the UI
    // (the mock agent's e2e:message emits an AGENT update, not a prompt).
    // Pin the ordinals first: the settled session must contain EXACTLY one
    // user message (the seeded first prompt), so the UI-sent prompt is
    // deterministically #2 and the seed #1.
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.filter((m) => m.author_type === "user").length;
      })
      .toBe(1);
    await session.clickSessionChatTab();
    await session.sendMessage(longPrompt);
    await expect
      .poll(settled, {
        timeout: 45_000,
        message: "Waiting for the long prompt's turn to settle",
      })
      .toBe(true);

    // Capture the persisted sentinel message id so the transcript-jump
    // locator targets exactly #msg-<capturedId> instead of matching by text
    // (the sentinel also renders in the prompt-history rows).
    let sentinelMessageId: string | null = null;
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        const match = messages.find(
          (m) => m.author_type === "user" && m.content.includes(SENTINEL),
        );
        if (match) sentinelMessageId = match.id;
        return Boolean(match);
      })
      .toBe(true);
    if (!sentinelMessageId) throw new Error("sentinel prompt was not persisted");

    // (3) Re-activate Prompt history: two rows, newest first, numbered #2/#1.
    await session.clickTab("Prompt history");
    const sentinelRow = testPage.getByTestId("prompt-history-row-0");
    const seededRow = testPage.getByTestId("prompt-history-row-1");
    await expect(sentinelRow).toContainText(SENTINEL);
    await expect(seededRow).toContainText(seedPrompt);
    await expect(sentinelRow.getByTestId("prompt-history-number-0")).toHaveText("#2");
    await expect(seededRow.getByTestId("prompt-history-number-1")).toHaveText("#1");

    // (4) Durations match formatPromptDuration shapes on both rows.
    const sentinelDuration = await sentinelRow
      .getByTestId("prompt-history-duration-0")
      .textContent();
    const seededDuration = await seededRow.getByTestId("prompt-history-duration-1").textContent();
    expect(sentinelDuration ?? "").toMatch(DURATION_SHAPE);
    expect(seededDuration ?? "").toMatch(DURATION_SHAPE);

    // (5) Expansion: the long prompt overflows (chevron visible), the box
    // scrolls internally, and its max-height equals 40% of the panel root.
    await sentinelRow.getByTestId("prompt-history-expand-0").click();
    const expandedBox = testPage.getByTestId("prompt-history-expanded-box-0");
    await expect(expandedBox).toBeVisible();
    const overflow = await expandedBox.evaluate((el) => el.scrollHeight > el.clientHeight);
    expect(overflow).toBe(true);
    const maxHeight = await expandedBox.evaluate((el) => {
      const root = document.querySelector('[data-testid="prompt-history-panel"]');
      return {
        computed: parseFloat(getComputedStyle(el).maxHeight) || 0,
        expected: Math.round((root?.clientHeight ?? 0) * 0.4),
      };
    });
    expect(Math.abs(maxHeight.computed - maxHeight.expected)).toBeLessThanOrEqual(2);

    // (6) Transcript jump: scroll the transcript AWAY (to the bottom — the
    // anchored last-prompt bar reserves a tall scroll-margin at the top, so a
    // top-anchored target near the top could not be aligned), re-activate
    // history, click the prompt bubble, and assert the sentinel prompt row is
    // top-aligned in the ACTIVE chat container (portal-mounted inactive
    // panels duplicate the rows, so everything must be scoped to the visible
    // chat).
    await session.clickSessionChatTab();
    const chat = session.activeChat();
    const messageList = chat.locator(".chat-message-list").first();
    await messageList.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    await session.clickTab("Prompt history");
    await sentinelRow.locator("[data-message-id]").click();

    await expect(session.activeChat()).toBeVisible();
    const msg = chat.locator(`#msg-${sentinelMessageId}`);
    await expect(msg).toHaveCount(1);
    await expect(msg).toBeAttached();

    // Scroll-settle poll on the transcript viewport: sample scrollTop until
    // it is unchanged across consecutive reads, then measure the settled
    // geometry (an immediate measurement can observe a transient position).
    let lastScrollTop = -1;
    let stableReads = 0;
    await expect
      .poll(
        async () => {
          const current = await messageList.evaluate((el) => el.scrollTop);
          if (current === lastScrollTop) stableReads += 1;
          else stableReads = 0;
          lastScrollTop = current;
          return stableReads;
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(2);

    // Top alignment: the row's top relative to its scrollport equals its
    // computed scroll-margin-top (dynamic via --anchored-bar-h), within a
    // small documented tolerance.
    const targetMargin = await msg.evaluate(
      (el) => parseFloat(getComputedStyle(el).scrollMarginTop) || 0,
    );
    const { elTop, listTop } = await msg.evaluate((el) => {
      const list = el.closest(".chat-message-list");
      const listRect = list?.getBoundingClientRect();
      const rect = el.getBoundingClientRect();
      return { elTop: rect.top, listTop: listRect?.top ?? 0 };
    });
    expect(Math.abs(elTop - listTop - targetMargin)).toBeLessThanOrEqual(2);
  });
  test("loads and aligns a genuinely unloaded middle prompt on desktop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const taskId = await seedLongPromptHistory(apiClient, {
      workspaceId: seedData.workspaceId,
      agentProfileId: seedData.agentProfileId,
      workflowId: seedData.workflowId,
      startStepId: seedData.startStepId,
      repositoryId: seedData.repositoryId,
    });

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await session.addPanelButton().click();
    await testPage.getByTestId("add-panel-prompt-history-item").click();

    const panel = testPage.getByTestId("prompt-history-panel");
    await expect(panel).toBeVisible();
    const middleRow = panel.locator('[data-testid^="prompt-history-row-"]', {
      hasText: MIDDLE_PROMPT_MARKER,
    });
    await revealPromptHistoryTarget(panel, middleRow);
    const messageId = await middleRow.locator("[data-message-id]").getAttribute("data-message-id");
    if (!messageId) throw new Error("Middle prompt row has no message id");
    await expect(testPage.locator(`#msg-${messageId}`)).toHaveCount(0);

    let aroundRequests = 0;
    await testPage.route("**/api/v1/task-sessions/*/messages*", async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get("around") === messageId) aroundRequests += 1;
      await route.continue();
    });
    await middleRow.locator("[data-message-id]").click();

    await expect.poll(() => aroundRequests, { timeout: 10_000 }).toBe(1);
    const chat = session.activeChat();
    await expect(chat).toBeVisible();
    const target = chat.locator(`#msg-${messageId}`);
    await expect(target).toBeAttached({ timeout: 10_000 });
    const messageList = chat.locator(".chat-message-list").first();
    await expect
      .poll(
        async () => {
          const targetTop = await target.evaluate((element) => element.getBoundingClientRect().top);
          const listTop = await messageList.evaluate(
            (element) => element.getBoundingClientRect().top,
          );
          const margin = await target.evaluate(
            (element) => parseFloat(getComputedStyle(element).scrollMarginTop) || 0,
          );
          return Math.abs(targetTop - listTop - margin);
        },
        { timeout: 10_000 },
      )
      .toBeLessThanOrEqual(2);
    expect(aroundRequests).toBe(1);
  });
});
