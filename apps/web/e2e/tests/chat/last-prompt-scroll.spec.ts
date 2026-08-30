// The task transcript can grow long enough that the user's own prompts scroll
// out of view. The "scroll-to-last-prompt" button appears only after the most
// recent prompt has cleared the transcript's settle tolerance, while
// "scroll-to-start" appears once the first prompt is no longer fully visible.
// The opt-in desktop anchored prompt bar additionally sticks a shortened copy
// of the last prompt under the view tab selector, with expand + the same
// scroll-up action.
import { test, expect } from "../../fixtures/test-base";
import {
  FIRST_PROMPT_MARKER,
  LAST_PROMPT_MARKER,
  seedScrolledPastLastPrompt,
} from "./last-prompt-scroll-helpers";

test.describe("@chat last prompt scroll affordance", () => {
  test.afterEach(async ({ apiClient }) => {
    // The anchored-bar test flips this setting; restore the default so later
    // tests in this worker see it again.
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: false,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: false,
      show_transcript_auto_scroll_control: true,
    });
  });

  test("scroll-to-last-prompt button jumps back to the top of the last prompt", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "last-prompt-scroll-button",
    );
    const chat = session.activeChat();
    const marker = chat.getByText(LAST_PROMPT_MARKER, { exact: false }).first();
    await expect(marker).not.toBeInViewport();

    const button = chat.getByTestId("scroll-to-last-prompt-button");
    await expect(button).toBeVisible();
    // The anchored bar is off by default.
    await expect(chat.getByTestId("anchored-last-prompt-bar")).toHaveCount(0);

    await button.click();
    await expect(marker).toBeInViewport({ timeout: 10_000 });
    await expect(button).toBeHidden({ timeout: 10_000 });
  });

  test("hides disabled transcript navigation controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await apiClient.saveUserSettings({
      show_scroll_to_last_prompt: false,
      show_scroll_to_start: false,
    });
    const { settings } = await apiClient.getUserSettings();
    expect(settings.show_scroll_to_last_prompt).toBe(false);
    expect(settings.show_scroll_to_start).toBe(false);
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "disabled-transcript-navigation",
      { showScrollControls: false },
    );
    const chat = session.activeChat();

    await expect(chat.getByTestId("scroll-to-last-prompt-button")).toHaveCount(0);
    await expect(chat.getByTestId("scroll-to-start-button")).toHaveCount(0);
  });

  test("keeps task opening bounded before last-prompt navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "paginated-last-prompt-scroll",
      { trailingFillerCount: 80 },
    );
    const olderPageRequests: string[] = [];
    testPage.on("request", (request) => {
      const url = new URL(request.url());
      if (
        url.pathname.includes("/task-sessions/") &&
        url.pathname.endsWith("/messages") &&
        url.searchParams.has("before")
      ) {
        olderPageRequests.push(url.toString());
      }
    });

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    const marker = chat.getByText(LAST_PROMPT_MARKER, { exact: false }).first();

    await expect(marker).not.toBeInViewport();
    const button = chat.getByTestId("scroll-to-last-prompt-button");
    await expect(button).toBeVisible({ timeout: 15_000 });
    // Opening the task must not fetch older pages just to locate the prompt.
    expect(olderPageRequests).toHaveLength(0);
    await expect(chat.getByText(FIRST_PROMPT_MARKER, { exact: false })).toHaveCount(0);
    await button.click();
    await expect(marker).toBeInViewport({ timeout: 10_000 });
  });

  test("scroll-to-start button jumps back to the first prompt", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "last-prompt-scroll-start",
    );
    const chat = session.activeChat();
    const firstMarker = chat.getByText(FIRST_PROMPT_MARKER, { exact: false }).first();
    await expect(firstMarker).not.toBeInViewport();

    const button = chat.getByTestId("scroll-to-start-button");
    await expect(button).toBeVisible();

    await button.click();
    await expect(firstMarker).toBeInViewport({ timeout: 10_000 });

    // Scrolling to the start hides the button again — the first message is
    // now fully visible, so there's nothing left to jump back to.
    await expect(button).toBeHidden({ timeout: 10_000 });
  });

  test("scroll-to-start drains older pages until it finds the true first prompt", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    // A large trailing filler pushes even the last prompt beyond the initial
    // fetch window (see the analogous last-prompt test above) — the first
    // prompt sits further back still, so reaching it requires draining
    // several older pages, not just one.
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "paginated-scroll-to-start",
      { trailingFillerCount: 80 },
    );
    const olderPageRequests: string[] = [];
    testPage.on("request", (request) => {
      const url = new URL(request.url());
      if (
        url.pathname.includes("/task-sessions/") &&
        url.pathname.endsWith("/messages") &&
        url.searchParams.has("before")
      ) {
        olderPageRequests.push(url.toString());
      }
    });

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const chat = session.activeChat();
    // Scoped to the transcript row itself (not `.first()` over bare text):
    // the mock agent's reply to the seeded prompt quotes it back verbatim,
    // so a loose text match could resolve to that echo instead of the real
    // user message once both are loaded post-drain.
    const firstMarker = chat
      .locator("[id^='msg-']")
      .filter({ has: testPage.getByText(FIRST_PROMPT_MARKER, { exact: true }) });

    // The initial fetch window doesn't reach back far enough to include it.
    await expect(firstMarker).toHaveCount(0);
    const button = chat.getByTestId("scroll-to-start-button");
    await expect(button).toBeVisible({ timeout: 15_000 });

    // Regression: clicking used to jump straight to the oldest message in
    // whatever page happened to be loaded — a partial-page boundary, not the
    // transcript's real start — because `hasMore` was never consulted first.
    await button.click();
    await expect(firstMarker).toBeInViewport({ timeout: 20_000 });
    expect(olderPageRequests.length).toBeGreaterThan(1);
    await expect(button).toBeHidden({ timeout: 10_000 });
  });

  test("anchored bar shows the last prompt while scrolled past it, expands, and scrolls back up", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await apiClient.saveUserSettings({ show_anchored_prompt_bar: true });
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "last-prompt-anchored-bar",
    );
    const chat = session.activeChat();

    // The footer action remains available until the anchored bar scrolls the
    // prompt itself back into view.
    const footerButton = chat
      .getByTestId("chat-status-bar")
      .getByTestId("scroll-to-last-prompt-button");
    await expect(footerButton).toBeVisible();

    const bar = chat.getByTestId("anchored-last-prompt-bar");
    await expect(bar).toHaveAttribute("data-state", "open", { timeout: 10_000 });

    const expandButton = bar.getByTestId("anchored-last-prompt-expand");
    await expect(expandButton).toHaveAttribute("aria-expanded", "false");
    await expandButton.click();
    await expect(expandButton).toHaveAttribute("aria-expanded", "true");
    await expect(bar.getByTestId("anchored-last-prompt-text")).toContainText(LAST_PROMPT_MARKER);

    await bar.getByTestId("scroll-to-last-prompt-button").click();
    await expect(bar).toHaveAttribute("data-state", "closed", { timeout: 10_000 });
    await expect(footerButton).toBeHidden({ timeout: 10_000 });
  });

  test("anchored bar preserves Markdown from the pinned user prompt", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const richPrompt = `${LAST_PROMPT_MARKER}\n\nRun \`terraform apply\` after review.`;
    await apiClient.saveUserSettings({ show_anchored_prompt_bar: true });
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "last-prompt-anchored-markdown",
      { lastPromptText: richPrompt },
    );
    const bar = session.activeChat().getByTestId("anchored-last-prompt-bar");

    await expect(bar).toHaveAttribute("data-state", "open", { timeout: 10_000 });
    await expect(bar.locator("code")).toHaveText("terraform apply");
  });

  test("anchored bar stays closed while the last prompt is partially visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await apiClient.saveUserSettings({ show_anchored_prompt_bar: true });
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "last-prompt-partial-clip",
    );
    const chat = session.activeChat();
    // Scope to the transcript row itself: with the anchored bar enabled, a
    // shortened copy of the same marker text also renders in the sticky bar
    // (always in viewport by design), and the mock agent's reply quotes the
    // marker back too. Require an exact-text descendant so only the user's
    // own prompt row — never the sticky bar or the agent's quoting reply —
    // satisfies the assertions below.
    const marker = chat
      .locator("[id^='msg-']")
      .filter({ has: testPage.getByText(LAST_PROMPT_MARKER, { exact: true }) });
    await expect(marker).toHaveCount(1);
    await expect(marker).toHaveAttribute("id", /^msg-.+/);
    const scrollButton = chat
      .getByTestId("chat-status-bar")
      .getByTestId("scroll-to-last-prompt-button");
    await scrollButton.click();
    await expect(marker).toBeInViewport({ timeout: 10_000 });
    // The scroll is animated, so `toBeInViewport` can pass mid-flight. Wait
    // for the container's scrollTop to stop changing before sampling geometry
    // from it, rather than assuming the animation fits in a fixed budget.
    let previousScrollTop = Number.NaN;
    await expect
      .poll(
        async () => {
          const current = await chat.evaluate(
            (root) =>
              root.querySelector<HTMLElement>(".chat-message-list")?.scrollTop ?? Number.NaN,
          );
          const settled = current === previousScrollTop;
          previousScrollTop = current;
          return settled;
        },
        { timeout: 10_000, message: "scroll animation did not settle" },
      )
      .toBe(true);

    const partialGeometry = await chat.evaluate((root, markerText) => {
      const scrollContainer = root.querySelector<HTMLElement>(".chat-message-list");
      const target = Array.from(root.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
        (element) => element.textContent?.includes(markerText),
      );
      if (!scrollContainer || !target) throw new Error("last prompt row is unavailable");
      scrollContainer.scrollTop +=
        target.getBoundingClientRect().top - scrollContainer.getBoundingClientRect().top + 3;
      scrollContainer.dispatchEvent(new Event("scroll"));
      return {
        containerTop: scrollContainer.getBoundingClientRect().top,
        targetTop: target.getBoundingClientRect().top,
        targetBottom: target.getBoundingClientRect().bottom,
      };
    }, LAST_PROMPT_MARKER);

    expect(partialGeometry.targetTop).toBeLessThan(partialGeometry.containerTop);
    expect(partialGeometry.targetBottom).toBeGreaterThan(partialGeometry.containerTop);
    await expect(chat.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
      "data-state",
      "closed",
      { timeout: 10_000 },
    );

    const fullyOutOfViewGeometry = await chat.evaluate((root, markerText) => {
      const scrollContainer = root.querySelector<HTMLElement>(".chat-message-list");
      const target = Array.from(root.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
        (element) => element.textContent?.includes(markerText),
      );
      if (!scrollContainer || !target) throw new Error("last prompt row is unavailable");
      scrollContainer.scrollTop +=
        target.getBoundingClientRect().bottom - scrollContainer.getBoundingClientRect().top + 3;
      scrollContainer.dispatchEvent(new Event("scroll"));
      return {
        containerTop: scrollContainer.getBoundingClientRect().top,
        targetBottom: target.getBoundingClientRect().bottom,
      };
    }, LAST_PROMPT_MARKER);

    expect(fullyOutOfViewGeometry.targetBottom).toBeLessThan(fullyOutOfViewGeometry.containerTop);
    await expect(chat.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
      "data-state",
      "open",
      { timeout: 10_000 },
    );
  });

  test("anchored bar closes and the scroll button points down while browsing history above the last prompt", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await apiClient.saveUserSettings({ show_anchored_prompt_bar: true });
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "last-prompt-below-state",
    );
    const chat = session.activeChat();
    const bar = chat.getByTestId("anchored-last-prompt-bar");
    const footerButton = chat
      .getByTestId("chat-status-bar")
      .getByTestId("scroll-to-last-prompt-button");

    // Seeding leaves the transcript scrolled past the last prompt (it sits
    // above the viewport): the bar is open and the button points up.
    await expect(bar).toHaveAttribute("data-state", "open", { timeout: 10_000 });
    await expect(footerButton.locator("svg")).toHaveClass(/tabler-icon-arrow-up/);
    await prCapture.screenshot("directional-above-state", {
      caption: "Scrolled past the last prompt: anchored bar open, up arrow",
    });
    await prCapture.screenshot("directional-up-arrow", {
      caption: "Scroll-to-last-prompt control pointing up",
    });

    // Jump to the very start of the transcript: the last prompt now sits
    // below the viewport (not yet reached) rather than above it (already
    // passed) — the anchored bar must stay closed, and the scroll button
    // must point the way the transcript will actually move: down.
    const marker = chat
      .locator("[id^='msg-']")
      .filter({ has: testPage.getByText(LAST_PROMPT_MARKER, { exact: true }) });
    await chat.getByTestId("scroll-to-start-button").click();
    await expect(chat.getByText(FIRST_PROMPT_MARKER, { exact: false }).first()).toBeInViewport({
      timeout: 10_000,
    });
    await expect(marker).not.toBeInViewport();

    await expect(bar).toHaveAttribute("data-state", "closed", { timeout: 10_000 });
    await expect(footerButton).toBeVisible();
    await expect(footerButton.locator("svg")).toHaveClass(/tabler-icon-arrow-down/);
    await prCapture.screenshot("directional-below-state", {
      caption: "Browsing history above the last prompt: anchored bar closed, down arrow",
    });
    await prCapture.screenshot("directional-down-arrow", {
      caption: "Scroll-to-last-prompt control pointing down",
    });

    // The action still scrolls to the last prompt, regardless of direction.
    await footerButton.click();
    await expect(marker).toBeInViewport({ timeout: 10_000 });
  });

  test("scroll-to-start survives a message streaming in mid-scroll", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    let sessionId = "";
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "streaming-scroll-race",
      { onSessionId: (id) => (sessionId = id) },
    );
    const chat = session.activeChat();
    const firstMarker = chat
      .locator("[id^='msg-']")
      .filter({ has: testPage.getByText(FIRST_PROMPT_MARKER, { exact: true }) });

    // Await the click so the scroll-to-start action has definitely started
    // (scrollIntoView kicks off the animation and returns immediately), then
    // fire a streamed message right away, no intervening wait — landing
    // while the smooth scroll is still animating. This used to race
    // useAutoScroll's follow-bottom effect: it would read a stale "near
    // bottom" state and snap the transcript straight back down, silently
    // cancelling the scroll-to-start action.
    await chat.getByTestId("scroll-to-start-button").click();
    await apiClient.seedAgentMessages(sessionId, 1, "race filler");

    await expect(firstMarker).toBeInViewport({ timeout: 10_000 });
    const isAtBottom = await chat.evaluate((root) => {
      const el = root.querySelector<HTMLElement>(".chat-message-list");
      if (!el) return false;
      return el.scrollHeight - el.scrollTop - el.clientHeight < 5;
    });
    expect(isAtBottom).toBe(false);
  });

  test("expanded anchored prompt caps at 40% of the transcript panel height", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 1440, height: 1200 });
    await apiClient.saveUserSettings({ show_anchored_prompt_bar: true });
    const longPrompt = Array.from(
      { length: 20 },
      (_, i) => `${LAST_PROMPT_MARKER} Line ${i + 1} of a deliberately long prompt.`,
    ).join("\n\n");
    const session = await seedScrolledPastLastPrompt(
      testPage,
      apiClient,
      seedData,
      "anchored-bar-proportional-height",
      { lastPromptText: longPrompt },
    );
    const chat = session.activeChat();
    const bar = chat.getByTestId("anchored-last-prompt-bar");
    await expect(bar).toHaveAttribute("data-state", "open", { timeout: 10_000 });

    const expandButton = bar.getByTestId("anchored-last-prompt-expand");
    await expandButton.click();
    const textEl = bar.getByTestId("anchored-last-prompt-text");
    await expect(textEl).toHaveAttribute("data-expanded", "true");

    // Proportional evidence: the expanded cell's height must track the
    // transcript panel's own height (~40% of it), not a fixed pixel value
    // that would look identical regardless of viewport size.
    const [textBox, panelHeight] = await Promise.all([
      textEl.boundingBox(),
      chat.locator(".chat-message-list").evaluate((el) => el.clientHeight),
    ]);
    expect(textBox).not.toBeNull();
    const expectedHeight = panelHeight * 0.4;
    expect(textBox!.height).toBeGreaterThan(expectedHeight * 0.85);
    expect(textBox!.height).toBeLessThan(expectedHeight * 1.15);

    await prCapture.screenshot("anchored-expanded-proportional-height", {
      caption: `Expanded anchored prompt capped at 40% of the transcript panel height (${Math.round(expectedHeight)}px of a ${panelHeight}px panel), not a fixed size`,
    });
  });
});
