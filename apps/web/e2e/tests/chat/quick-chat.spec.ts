import { type Locator, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { attachAvailableCommandsCapture, attachShellInputCapture } from "../../helpers/ws-capture";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import {
  openQuickChatSetup,
  openQuickChatWithAgent,
  quickChatTabReferences,
  selectAgentIfNeeded,
  sendQuickChatMessage,
  startQuickChatFromSetup,
} from "./quick-chat-helpers";

/**
 * Quick Chat E2E tests: basic flow, enhance prompt, queued messages, multi-tab.
 */

async function waitForQuickChatWidth(dialog: Locator) {
  await expect
    .poll(() =>
      dialog.evaluate((element) => {
        const preferred = Number.parseFloat(
          getComputedStyle(element).getPropertyValue("--quick-chat-width"),
        );
        return Math.abs(element.getBoundingClientRect().width - preferred);
      }),
    )
    .toBeLessThan(2);
}

async function dragQuickChatTab(page: Page, source: Locator, target: Locator) {
  const [sourceBox, targetBox] = await Promise.all([
    source
      .locator('[data-testid="quick-chat-tab"], [data-testid="quick-terminal-tab"]')
      .first()
      .boundingBox(),
    target.boundingBox(),
  ]);
  expect(sourceBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  if (!sourceBox || !targetBox) throw new Error("quick chat tab geometry unavailable");

  const sourceX = sourceBox.x + sourceBox.width / 2;
  const sourceY = sourceBox.y + sourceBox.height / 2;
  await page.mouse.move(sourceX, sourceY);
  await page.mouse.down();
  await page.mouse.move(sourceX + 16, sourceY);
  await page.mouse.move(targetBox.x + targetBox.width / 2, targetBox.y + targetBox.height / 2, {
    steps: 8,
  });
  await page.mouse.up();
}

test.describe("Quick Chat", () => {
  // @covers AC-UI-QUICK-CHAT-ELEVATION-001.1 AC-UI-QUICK-CHAT-ELEVATION-001.2 AC-UI-QUICK-CHAT-ELEVATION-001.5 AC-UI-QUICK-CHAT-ELEVATION-001.6
  test("adds a stronger elevation backdrop when opened over the page", async ({ testPage }) => {
    const dialog = await openQuickChatSetup(testPage);
    const overlay = testPage.locator('[data-slot="dialog-overlay"]');

    await expect(overlay).toBeVisible();
    const styles = await Promise.all([
      overlay.evaluate((element) => getComputedStyle(element).backgroundColor),
      overlay.evaluate((element) => getComputedStyle(element).backdropFilter),
      dialog.evaluate((element) => getComputedStyle(element).boxShadow),
    ]);
    expect(styles[0]).not.toBe("rgba(0, 0, 0, 0)");
    expect(styles[0]).not.toBe("transparent");
    expect(styles[1]).not.toBe("none");
    expect(styles[2]).not.toBe("none");

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await expect(overlay).not.toBeVisible();
  });

  test("returns focus without a visible indicator", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    const dialog = await openQuickChatSetup(testPage, false);
    const launcher = testPage.getByTestId("sidebar-quick-chat-shortcut");

    await testPage.keyboard.press("Escape");

    await expect(dialog).not.toBeVisible();
    await expect(launcher).toBeFocused();
    await expect(launcher).toHaveAttribute("data-quick-chat-silent-focus", "true");

    const silentStyles = await launcher.evaluate((element) => {
      const styles = getComputedStyle(element);
      return {
        outlineStyle: styles.outlineStyle,
        boxShadow: styles.boxShadow,
        borderColor: styles.borderColor,
      };
    });
    expect(silentStyles.outlineStyle).toBe("none");
    expect(silentStyles.boxShadow).toBe("none");
    expect(silentStyles.borderColor).toBe("rgba(0, 0, 0, 0)");

    await testPage.getByTestId("create-task-button").focus();
    await expect(launcher).not.toHaveAttribute("data-quick-chat-silent-focus");

    await testPage.keyboard.press("Tab");
    await testPage.keyboard.press("Tab");
    await expect(launcher).toBeFocused();

    const normalFocusStyles = await launcher.evaluate((element) => {
      const styles = getComputedStyle(element);
      return {
        outlineStyle: styles.outlineStyle,
        boxShadow: styles.boxShadow,
        borderColor: styles.borderColor,
      };
    });
    expect(normalFocusStyles.outlineStyle).not.toBe("none");
    expect(normalFocusStyles.borderColor).not.toBe(silentStyles.borderColor);
  });

  test("returns focus to a shortcut origin without a silent marker", async ({ testPage }) => {
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    const origin = testPage.getByTestId("create-task-button");
    await origin.focus();

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+q`);
    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog).toBeVisible();

    await testPage.keyboard.press("Escape");

    await expect(dialog).not.toBeVisible();
    await expect(origin).toBeFocused();
    await expect(origin).not.toHaveAttribute("data-quick-chat-silent-focus");
  });

  test("@covers AC-UI-QUICK-TERMINAL-001.10 AC-UI-QUICK-TERMINAL-001.11 passthrough Escape stays in the TUI and the shortcut closes Quick Chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const capture = attachShellInputCapture(testPage);
    const { agents } = await apiClient.listAgents();
    const profileOwner = agents.find((agent) =>
      agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
    );
    if (!profileOwner) throw new Error("Seed agent profile owner was not found");

    let profileId: string | null = null;

    try {
      const profile = await apiClient.createAgentProfile(profileOwner.id, "Quick Chat Escape TUI", {
        model: "mock-fast",
        cli_passthrough: true,
      });
      profileId = profile.id;
      await apiClient.updateWorkspace(seedData.workspaceId, {
        default_agent_profile_id: profile.id,
      });

      const dialog = await openQuickChatSetup(testPage);
      await dialog.getByTestId("quick-chat-start").click();

      const terminal = dialog.getByTestId("passthrough-terminal");
      await expect(terminal).toBeVisible({ timeout: 15_000 });
      await expect(terminal.getByTestId("passthrough-loading")).not.toBeVisible({
        timeout: 30_000,
      });
      await terminal.click();
      await expect(terminal.locator(".xterm-helper-textarea")).toBeFocused();

      capture.frames.length = 0;
      await testPage.keyboard.press("Escape");

      await expect(dialog).toBeVisible();
      await expect
        .poll(() => capture.frames.some((frame) => frame.data === "\x1b"), {
          message: "Expected xterm to forward Escape to the passthrough terminal WebSocket",
        })
        .toBe(true);

      const modifier = process.platform === "darwin" ? "Meta" : "Control";
      const quickChatShortcut = `${modifier}+Shift+${SHORTCUTS.QUICK_CHAT.key}`;
      await testPage.keyboard.press(quickChatShortcut);
      await expect(dialog).not.toBeVisible();
    } finally {
      try {
        await apiClient.updateWorkspace(seedData.workspaceId, {
          default_agent_profile_id: seedData.agentProfileId,
        });
      } finally {
        if (profileId) await apiClient.cleanupTestProfiles([profileId]);
      }
    }
  });

  test("clarification shortcuts work after clicking the message surface", async ({ testPage }) => {
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-multi");

    const clarification = dialog.getByTestId("clarification-overlay");
    await expect(clarification).toBeVisible({ timeout: 30_000 });

    await dialog.getByTestId("quick-chat-messages").click({ position: { x: 8, y: 8 } });
    await expect(dialog.getByTestId("quick-chat-content")).toBeFocused();

    await testPage.keyboard.press("1");
    await expect(
      clarification.locator('[data-testid="clarification-step"][data-step-index="1"]'),
    ).toHaveAttribute("data-active", "true");
  });

  test("Escape collapses the clarification panel before closing the modal", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-multi");

    const clarification = dialog.getByTestId("clarification-overlay");
    await expect(clarification).toBeVisible({ timeout: 30_000 });

    const bar = dialog.getByTestId("clarification-overlay-container");

    // The collapse shortcut only fires for keydowns targeting the shortcut
    // scope (quick-chat-content), same as the numeric-step shortcut above.
    await dialog.getByTestId("quick-chat-messages").click({ position: { x: 8, y: 8 } });
    await expect(dialog.getByTestId("quick-chat-content")).toBeFocused();

    await testPage.keyboard.press("Escape");

    // First Escape: a pending, expanded clarification collapses in place and
    // the modal stays open, matching the main task chat panel's two-stage
    // Escape after #2729 — this is what distinguishes it from Radix's default
    // (close-on-Escape) DismissableLayer behavior.
    await expect(dialog).toBeVisible();
    await expect(clarification).not.toBeVisible();
    await expect(bar).toBeVisible();

    await testPage.keyboard.press("Escape");

    // Second Escape is now unguarded: it closes the whole modal.
    await expect(dialog).not.toBeVisible();
  });

  test("Escape collapses the clarification panel with focus left in the composer after sending", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-multi");

    const clarification = dialog.getByTestId("clarification-overlay");
    await expect(clarification).toBeVisible({ timeout: 30_000 });

    // Focus the composer directly instead of an inert part of the message
    // list: this is the ordinary post-send state, and the composer is an
    // editable target the collapse shortcut must still claim Escape for.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await expect(editor).toBeFocused();

    await testPage.keyboard.press("Escape");

    await expect(dialog).toBeVisible();
    await expect(clarification).not.toBeVisible();
    await expect(dialog.getByTestId("clarification-overlay-container")).toBeVisible();
  });

  test("Escape closes the modal immediately when focus is outside the clarification's shortcut scope", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-multi");

    const clarification = dialog.getByTestId("clarification-overlay");
    await expect(clarification).toBeVisible({ timeout: 30_000 });

    // The tab bar sits outside quick-chat-content (the clarification's
    // shortcut scope), e.g. the resize handles or a mouse-focused tab.
    await dialog
      .locator('[data-testid="quick-chat-tab"] button[title="Double-click to rename"]')
      .focus();

    await testPage.keyboard.press("Escape");

    // A pending, still-expanded clarification does not block Escape here: the
    // guard predicate only claims Escape for keydowns inside its scope.
    await expect(dialog).not.toBeVisible();
  });

  test("Escape closes the open entity-reference suggestion popup without collapsing the clarification panel", async ({
    testPage,
  }) => {
    // Regression coverage for the real ProseMirror Suggestion keydown path:
    // earlier rounds only exercised a synthetic in-scope listener, which
    // stayed green while Radix's Dialog capture-phase preventDefault() on
    // Escape silently starved prosemirror-view's own keydown handling (and
    // therefore the Suggestion plugin's onKeyDown) on Quick Chat specifically.
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-multi");

    const clarification = dialog.getByTestId("clarification-overlay");
    await expect(clarification).toBeVisible({ timeout: 30_000 });
    const bar = dialog.getByTestId("clarification-overlay-container");

    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.pressSequentially("#");

    // PopupMenu always portals to document.body (see popup-menu.tsx), outside
    // the Dialog's own DOM subtree, so this must be queried at the page level
    // -- mirrors entity-reference-composer.spec.ts's non-Quick-Chat usage.
    const menu = testPage.getByTestId("entity-reference-menu");
    await expect(menu).toBeVisible({ timeout: 10_000 });

    await testPage.keyboard.press("Escape");

    // The suggestion popup owns this Escape: it closes, the clarification
    // panel stays exactly as it was (neither collapsed nor closed), and the
    // modal stays open -- the fallback in tiptap-input.tsx claimed the event
    // via stopPropagation() before Quick Chat's own carousel/collapse
    // listener ever saw it. clarification-overlay-container wraps both the
    // collapsed and expanded states with nonzero height, so it stays visible
    // throughout; `clarification` (clarification-overlay) is the element that
    // actually reflects collapsed vs. expanded.
    await expect(menu).not.toBeVisible();
    await expect(dialog).toBeVisible();
    await expect(clarification).toBeVisible();
    await expect(bar).toBeVisible();

    // Escape resumes its normal two-stage behavior on the next press: the
    // fallback only acts while a suggestion menu is open.
    await testPage.keyboard.press("Escape");
    await expect(clarification).not.toBeVisible();
    await expect(bar).toBeVisible();
    await expect(dialog).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
  });

  test("Escape closes the open entity-reference suggestion popup without closing the dialog when no clarification is pending", async ({
    testPage,
  }) => {
    // Companion to the clarification-pending case above. Radix's
    // DismissableLayer auto-dismisses the whole dialog on Escape unless
    // something already called preventDefault() during the capture phase --
    // with no clarification mounted, the carousel's own guard never arms, so
    // this proves the suggestion-menu-open guard registered in
    // use-suggestion-escape-fallback.ts is what keeps the dialog open here.
    const dialog = await openQuickChatWithAgent(testPage);

    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.pressSequentially("#");

    const menu = testPage.getByTestId("entity-reference-menu");
    await expect(menu).toBeVisible({ timeout: 10_000 });

    await testPage.keyboard.press("Escape");

    await expect(menu).not.toBeVisible();
    await expect(dialog).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
  });

  test("reverse-search overlay opens, filters by typing, and closes on Escape inside Quick Chat", async ({
    testPage,
  }) => {
    // Regression coverage for message-history-search.tsx portaling outside
    // the Dialog's FocusScope: previously the overlay's own focus() and
    // Escape/typing handlers never fired on Quick Chat because the Dialog's
    // focus trap reverted focus away from the document.body-portaled input.
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "first reverse search message");
    await expect(dialog.getByText("first reverse search message", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    // REVERSE_SEARCH defaults to plain Ctrl (not ctrlOrCmd) even on macOS, so
    // Cmd+R keeps triggering the browser's native refresh shortcut.
    await testPage.keyboard.press("Control+r");

    const overlay = dialog.getByTestId("history-search-overlay");
    await expect(overlay).toBeVisible({ timeout: 10_000 });
    const input = dialog.getByTestId("history-search-input");
    await expect(input).toBeFocused();

    await input.pressSequentially("reverse search");
    await expect(
      overlay.getByTestId("history-search-row").filter({ hasText: "first reverse search message" }),
    ).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(overlay).not.toBeVisible();
    await expect(dialog).toBeVisible();
  });

  test("offers configuration chat in setup and hides it once one exists", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });
    const dialog = await openQuickChatSetup(testPage);
    const setup = dialog.getByTestId("quick-chat-setup");

    await expect(setup.getByText(/quick chats stay outside your task board/i)).toBeVisible();
    await setup.getByRole("switch", { name: "Configuration chat" }).click();

    const configSetup = dialog.getByTestId("config-chat-setup");
    await expect(configSetup).toBeVisible();
    await expect(configSetup.getByRole("switch", { name: "Configuration chat" })).toBeChecked();
    await configSetup
      .getByPlaceholder("Ask anything about your configuration...")
      .fill("/e2e:simple-message");
    await configSetup.getByRole("button", { name: "Start configuration chat" }).click();
    await expect(configSetup).not.toBeVisible({ timeout: 15_000 });

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+q`);
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    const configTab = dialog
      .getByTestId("quick-chat-tab")
      .filter({ has: testPage.getByRole("img", { name: "Configuration chat" }) });
    await configTab
      .getByRole("button", { name: /Configuration chat \/e2e:simple-message/ })
      .click();
    await expect(configTab).toHaveClass(/bg-background/);

    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await testPage.getByTestId("quick-chat-new-agent").click();
    const newSetup = dialog.getByTestId("quick-chat-setup");
    await expect(newSetup).toBeVisible();
    await expect(newSetup.getByRole("switch", { name: "Configuration chat" })).toHaveCount(0);
  });

  test("resizes from either edge, restores width, and keeps tab actions adjacent", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatSetup(testPage);
    await waitForQuickChatWidth(dialog);
    const initialBox = await dialog.boundingBox();
    expect(initialBox).not.toBeNull();

    const rightHandle = dialog.getByTestId("quick-chat-resize-right");
    await expect(rightHandle).toBeVisible();
    const rightBox = await rightHandle.boundingBox();
    expect(rightBox).not.toBeNull();
    await testPage.mouse.move(
      rightBox!.x + rightBox!.width / 2,
      rightBox!.y + rightBox!.height / 2,
    );
    await testPage.mouse.down();
    await expect.poll(() => testPage.evaluate(() => document.body.style.cursor)).toBe("ew-resize");
    await testPage.mouse.move(
      rightBox!.x + rightBox!.width / 2 + 50,
      rightBox!.y + rightBox!.height / 2,
    );
    await testPage.mouse.up();
    await waitForQuickChatWidth(dialog);

    const rightResizedBox = await dialog.boundingBox();
    expect(rightResizedBox!.width).toBeGreaterThan(initialBox!.width + 80);

    const leftHandle = dialog.getByTestId("quick-chat-resize-left");
    const leftBox = await leftHandle.boundingBox();
    expect(leftBox).not.toBeNull();
    await leftHandle.hover();
    const leftHighlightBox = await leftHandle.locator("span").boundingBox();
    expect(leftHighlightBox).not.toBeNull();
    expect(leftHighlightBox!.x).toBeCloseTo(rightResizedBox!.x, 0);
    await testPage.mouse.move(leftBox!.x + leftBox!.width / 2, leftBox!.y + leftBox!.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(
      leftBox!.x + leftBox!.width / 2 - 40,
      leftBox!.y + leftBox!.height / 2,
    );
    await testPage.mouse.up();
    await waitForQuickChatWidth(dialog);

    const finalBox = await dialog.boundingBox();
    const finalPreferredWidth = await dialog.evaluate((element) =>
      Number.parseFloat(getComputedStyle(element).getPropertyValue("--quick-chat-width")),
    );
    expect(finalBox!.width).toBeGreaterThan(rightResizedBox!.width + 50);
    expect(finalBox!.x + finalBox!.width / 2).toBeCloseTo(
      (await testPage.evaluate(() => window.innerWidth)) / 2,
      0,
    );

    const tab = dialog.getByTestId("quick-chat-tab").last();
    const newChat = dialog.getByTestId("quick-chat-add-menu-trigger");
    const tabBox = await tab.boundingBox();
    const newChatBox = await newChat.boundingBox();
    expect(newChatBox!.x - (tabBox!.x + tabBox!.width)).toBeLessThanOrEqual(8);

    const setupSurfaces = await dialog.evaluate((element) => {
      const setup = element.querySelector<HTMLElement>('[data-testid="quick-chat-setup"]');
      const footer = element.querySelector<HTMLElement>('[data-testid="quick-chat-setup-footer"]');
      return {
        dialog: getComputedStyle(element).backgroundColor,
        setup: setup ? getComputedStyle(setup).backgroundColor : null,
        footer: footer ? getComputedStyle(footer).backgroundColor : null,
      };
    });
    expect(setupSurfaces.setup).toBe(setupSurfaces.dialog);
    expect(setupSurfaces.footer).toBe(setupSurfaces.setup);

    await startQuickChatFromSetup(dialog, testPage);
    const surfaces = await dialog.evaluate((element) => {
      const messages = element.querySelector<HTMLElement>('[data-testid="quick-chat-messages"]');
      const input = element.querySelector<HTMLElement>('[data-testid="chat-input-area"]');
      return {
        dialog: getComputedStyle(element).backgroundColor,
        messages: messages ? getComputedStyle(messages).backgroundColor : null,
        input: input ? getComputedStyle(input).backgroundColor : null,
      };
    });
    expect(surfaces.messages).toBe(surfaces.dialog);
    expect(surfaces.input).toBe(surfaces.messages);

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await testPage.getByTestId("sidebar-quick-chat-shortcut").click();
    await expect(dialog).toBeVisible();
    await waitForQuickChatWidth(dialog);
    const restoredBox = await dialog.boundingBox();
    const restoredPreferredWidth = await dialog.evaluate((element) =>
      Number.parseFloat(getComputedStyle(element).getPropertyValue("--quick-chat-width")),
    );
    expect(restoredPreferredWidth).toBe(finalPreferredWidth);
    expect(Math.abs(restoredBox!.width - finalBox!.width)).toBeLessThan(2);
  });

  test("explains quick chat and starts with repository context", async ({
    testPage,
    seedData,
    backend,
  }) => {
    const sourceRepo = path.join(backend.tmpDir, "repos", "e2e-repo");
    const contextBranch = "quick-chat-context-branch";
    execFileSync("git", ["branch", "-f", contextBranch], { cwd: sourceRepo });
    // Repository-backed quick chat performs a required refresh before it
    // materializes the worktree. Publish the local context branch to the
    // fixture's offline origin so that refresh succeeds deterministically.
    execFileSync("git", ["push", "--force", "origin", contextBranch], { cwd: sourceRepo });
    try {
      const dialog = await openQuickChatSetup(testPage);
      await expect(dialog.getByTestId("quick-chat-introduction")).toContainText(
        "Chat with an agent about an idea, question, or codebase.",
      );
      await expect(dialog.getByTestId("quick-chat-introduction")).toContainText(
        "Quick chats stay outside your task board.",
      );
      await expect(
        dialog.getByText("Add repository context to focus on specific code and branches."),
      ).toBeVisible();
      await selectAgentIfNeeded(dialog, testPage);

      await dialog.getByTestId("add-repository").click();
      await dialog.getByTestId("repo-chip-trigger").click();
      await testPage.getByRole("option").first().click();
      await dialog.getByTestId("branch-chip-trigger").click();
      await testPage.locator(`[role="option"][data-value="${contextBranch}"]`).click();

      const startRequest = testPage.waitForRequest(
        (request) => request.url().includes("/quick-chat") && request.method() === "POST",
      );
      await dialog.getByTestId("quick-chat-start").click();
      const payload = (await startRequest).postDataJSON() as {
        repositories?: Array<{ repository_id: string; base_branch: string }>;
      };
      expect(payload.repositories).toEqual([
        { repository_id: seedData.repositoryId, base_branch: contextBranch },
      ]);
      await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 30_000 });
      expect(
        execFileSync("git", ["branch", "--show-current"], {
          cwd: sourceRepo,
          encoding: "utf8",
        }).trim(),
      ).toBe("main");
    } finally {
      execFileSync("git", ["push", "origin", "--delete", contextBranch], { cwd: sourceRepo });
      execFileSync("git", ["branch", "-D", contextBranch], { cwd: sourceRepo });
    }
  });

  test("opens quick chat, selects agent, sends message and receives response", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatWithAgent(testPage);

    await sendQuickChatMessage(dialog, testPage, "/e2e:simple-message");

    // Mock agent scenario "simple-message" responds with this text.
    await expect(
      dialog.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
  });

  test("enhance prompt replaces input text with AI-enhanced version", async ({
    testPage,
    apiClient,
  }) => {
    // Configure utility agent so the enhance button is enabled.
    await apiClient.saveUserSettings({
      default_utility_agent_id: "mock",
      default_utility_model: "mock-fast",
    });

    // Intercept utility execute API to return mock enhanced text.
    await testPage.route("**/api/v1/utility/execute", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          response: "Enhanced: please fix the null pointer bug in the user service",
          model: "mock-fast",
          prompt_tokens: 50,
          response_tokens: 20,
          duration_ms: 100,
        }),
      });
    });

    const dialog = await openQuickChatWithAgent(testPage);

    // Type initial text. Re-gate on the editor being editable: eager init can
    // flip it back to contenteditable=false (agent briefly RUNNING) after the
    // open helper's initial check, and fill() requires an editable element.
    const editor = dialog.locator(".tiptap.ProseMirror");
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
    await editor.click();
    await editor.fill("fix the bug");

    // Click the enhance prompt button.
    const enhanceBtn = dialog.getByLabel("Enhance prompt with AI");
    await expect(enhanceBtn).toBeVisible({ timeout: 5_000 });
    await expect(enhanceBtn).toBeEnabled();
    await enhanceBtn.click();

    // Wait for enhanced text to replace input.
    await expect(editor).toHaveText(
      "Enhanced: please fix the null pointer bug in the user service",
      { timeout: 10_000 },
    );
  });

  test("keeps an edited active-session prompt and offers recovery", async ({
    testPage,
    apiClient,
  }) => {
    const initialPrompt = "Fix the original bug";
    const editedPrompt = "Fix the original bug with an added constraint";
    const generatedPrompt = "Enhanced: fix the original bug with an added constraint and tests";
    let requestStarted: (() => void) | undefined;
    let releaseResponse: (() => void) | undefined;
    const requestGate = new Promise<void>((resolve) => {
      requestStarted = resolve;
    });
    const responseGate = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });

    await apiClient.saveUserSettings({
      default_utility_agent_id: "mock",
      default_utility_model: "mock-fast",
    });
    await testPage.route("**/api/v1/utility/execute", async (route) => {
      requestStarted?.();
      await responseGate;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, response: generatedPrompt }),
      });
    });

    const dialog = await openQuickChatWithAgent(testPage);
    const editor = dialog.locator(".tiptap.ProseMirror");
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
    await editor.fill(initialPrompt);
    await dialog.getByLabel("Enhance prompt with AI").click();
    await requestGate;
    await editor.focus();
    await editor.pressSequentially(" with an added constraint");
    await expect(editor).toHaveText(editedPrompt);
    releaseResponse?.();

    const recovery = dialog.getByTestId("prompt-result-recovery");
    await expect(recovery).toBeVisible();
    await expect(editor).toHaveText(editedPrompt);

    await recovery.getByRole("button", { name: "Apply" }).click();
    await expect(editor).toHaveText(generatedPrompt);
  });

  test("slash command menu populates before first message (eager agent init)", async ({
    testPage,
  }) => {
    // Picking an agent in quick chat should boot the agent process eagerly,
    // so available_commands_update fires from session/new — the slash menu is
    // populated before the user sends their first prompt. Mock-agent emits
    // /slow, /error, /thinking, etc. on session/new (parity with real ACP
    // agents like OpenCode and Claude).
    const availableCommands = attachAvailableCommandsCapture(testPage);

    const dialog = await openQuickChatWithAgent(testPage);

    // Wait for the available_commands WS frame to land. Eager init kicks off
    // session/new during the HTTP request, but the agent emits commands
    // asynchronously after the response flushes — so the frame can arrive
    // moments after openQuickChatWithAgent resolves.
    await expect
      .poll(() => availableCommands.frames.some((frame) => frame.count > 0), { timeout: 15_000 })
      .toBe(true);

    const editor = dialog.locator(".tiptap.ProseMirror");
    await editor.click();
    await editor.pressSequentially("/");

    // SlashCommandMenu renders into a portal at document root, so query at page level.
    await expect(testPage.getByText("Commands").first()).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("/slow")).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByText("/error")).toBeVisible({ timeout: 5_000 });
  });

  test("model selector shows dynamic session options before first message", async ({
    testPage,
  }) => {
    const dialog = await openQuickChatWithAgent(testPage);

    const trigger = dialog.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });
    await trigger.click();

    const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(effortTrigger).toBeVisible({
      timeout: 10_000,
    });
    await effortTrigger.click();
    await expect(testPage.getByTestId("config-option-section-effort")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("exposes tab actions on a coarse-pointer touch tablet", async ({ tabletTestPage }) => {
    expect(await tabletTestPage.evaluate(() => window.innerWidth)).toBe(900);
    expect(
      await tabletTestPage.evaluate(() => window.matchMedia("(pointer: coarse)").matches),
    ).toBe(true);

    const dialog = await openQuickChatWithAgent(tabletTestPage);
    const actionTrigger = dialog.getByRole("button", { name: /^Actions for / });
    await expect(actionTrigger).toBeVisible();
    await expect(actionTrigger).toHaveClass(/h-11/);
    await expect(actionTrigger).toHaveClass(/w-11/);

    await actionTrigger.tap();
    await expect(
      tabletTestPage.getByRole("menuitem", { name: "Rename", exact: true }),
    ).toBeVisible();
    await expect(tabletTestPage.getByRole("menuitem", { name: /^Move .* left$/ })).toBeVisible();
    await expect(tabletTestPage.getByRole("menuitem", { name: /^Move .* right$/ })).toBeVisible();
    await expect(tabletTestPage.getByRole("menuitem", { name: /^Close .*$/ })).toBeVisible();
  });

  test("preserves mixed tab order, generated title, and rename after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const quickChatWorkspace = await apiClient.createWorkspace("Quick Chat Restore Workspace");
    const workflow = await apiClient.createWorkflow(
      quickChatWorkspace.id,
      "Restore Workflow",
      "simple",
    );
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const startStep = steps.find((step) => step.is_start_step) ?? steps[0];
    const task = await apiClient.createTaskWithAgent(
      quickChatWorkspace.id,
      "Quick Chat Restore Task",
      seedData.agentProfileId,
      {
        workflow_id: workflow.id,
        workflow_step_id: startStep.id,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    await testPage.waitForLoadState("networkidle");
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
    await expect
      .poll(() =>
        testPage.evaluate("window.__KANDEV_E2E_STORE__?.getState().workspaces.activeId ?? null"),
      )
      .toBe(quickChatWorkspace.id);

    await apiClient.saveUserSettings({ agent_generated_task_titles: true });
    const dialog = await openQuickChatSetup(testPage, false);

    const firstStart = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    await startQuickChatFromSetup(dialog, testPage);
    const first = (await (await firstStart).json()) as { session_id: string; task_id: string };
    const firstReference = `conversation:${first.session_id}`;

    // The deterministic script calls the real title MCP endpoint through the
    // mock agent, then the task.updated event relabels the open tab.
    const generatedTitle = "Investigate cache invalidation";
    await sendQuickChatMessage(
      dialog,
      testPage,
      `e2e:mcp:kandev:set_task_title_kandev(${JSON.stringify({ title: generatedTitle })})`,
    );
    const firstTab = dialog.locator(
      `[data-tab-reference="${firstReference}"] [data-testid="quick-chat-tab"]`,
    );
    await expect(firstTab.getByTestId("quick-chat-tab-name")).toHaveText(generatedTitle, {
      timeout: 30_000,
    });

    // Add a terminal between the first chat and the next chat.
    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await testPage.getByTestId("quick-chat-new-terminal").click();
    await expect(dialog.getByTestId("quick-terminal-tab")).toHaveCount(1, { timeout: 15_000 });
    await expect(dialog.getByTestId("quick-terminal-terminal")).toBeVisible({ timeout: 15_000 });
    const terminalSortable = dialog.locator(
      '[data-testid="quick-chat-sortable-tab"][data-tab-reference^="terminal:"]',
    );
    const terminalReference = await terminalSortable.getAttribute("data-tab-reference");
    if (!terminalReference) throw new Error("quick terminal reference was not rendered");

    // Create a second chat after the terminal. The stable baseline puts all
    // conversations first and terminals after them until the user reorders it.
    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await testPage.getByTestId("quick-chat-new-agent").click();

    // Setup guidance remains visible for every new chat.
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByTestId("quick-chat-introduction")).toBeVisible();

    const secondStart = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    await startQuickChatFromSetup(dialog, testPage);
    const second = (await (await secondStart).json()) as { session_id: string; task_id: string };
    const secondReference = `conversation:${second.session_id}`;
    await expect
      .poll(() => quickChatTabReferences(dialog), { timeout: 15_000 })
      .toEqual([firstReference, secondReference, terminalReference]);

    // Rename the second chat through the desktop context-menu path.
    const secondTab = dialog.locator(
      `[data-tab-reference="${secondReference}"] [data-testid="quick-chat-tab"]`,
    );
    await secondTab.locator('button[title="Double-click to rename"]').click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Rename", exact: true }).click();
    const renameInput = secondTab.getByRole("textbox", { name: "Rename chat" });
    await expect(renameInput).toBeVisible();
    await renameInput.fill("Cache follow-up");
    await renameInput.press("Enter");
    await expect(secondTab.getByTestId("quick-chat-tab-name")).toHaveText("Cache follow-up");

    // Drag the terminal ahead of both conversations. This exercises the real
    // pointer sensor and persists the mixed conversation/terminal order.
    await dragQuickChatTab(
      testPage,
      terminalSortable,
      dialog.locator(
        `[data-testid="quick-chat-sortable-tab"][data-tab-reference="${firstReference}"]`,
      ),
    );
    await expect
      .poll(() => quickChatTabReferences(dialog), { timeout: 15_000 })
      .toEqual([terminalReference, firstReference, secondReference]);

    const beforeReloadResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${quickChatWorkspace.id}/tasks?only_ephemeral=true&exclude_config=true`,
    );
    expect(beforeReloadResponse.ok).toBe(true);
    const beforeReload = (await beforeReloadResponse.json()) as {
      tasks: Array<{ id: string }>;
    };
    expect(beforeReload.tasks).toHaveLength(2);

    const terminalBeforeReloadResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/quick-terminal-tabs?workspace_id=${quickChatWorkspace.id}`,
    );
    expect(terminalBeforeReloadResponse.ok).toBe(true);
    const terminalsBeforeReload = (await terminalBeforeReloadResponse.json()) as {
      tabs: Array<{ tabId: string }>;
    };
    expect(terminalsBeforeReload.tabs.map((tab) => tab.tabId)).toEqual([
      terminalReference.replace("terminal:", ""),
    ]);

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");

    const persistedResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${quickChatWorkspace.id}/tasks?only_ephemeral=true&exclude_config=true`,
    );
    expect(persistedResponse.ok).toBe(true);
    const persisted = (await persistedResponse.json()) as { tasks: Array<{ id: string }> };
    expect(persisted.tasks).toHaveLength(2);

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+q`);
    const restoredDialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(restoredDialog).toBeVisible({ timeout: 10_000 });
    await expect
      .poll(() =>
        testPage.evaluate(
          "window.__KANDEV_E2E_STORE__?.getState().quickChat.terminalTabs?.length ?? -1",
        ),
      )
      .toBe(1);
    const restoredTerminalState = await testPage.evaluate(
      "JSON.stringify(window.__KANDEV_E2E_STORE__?.getState().quickChat.terminalTabs ?? [])",
    );
    expect(restoredTerminalState).toContain(terminalReference.replace("terminal:", ""));
    expect(restoredTerminalState).toContain(quickChatWorkspace.id);

    const restoredTabs = restoredDialog.getByTestId("quick-chat-sortable-tab");
    await expect(restoredTabs).toHaveCount(3);
    await expect
      .poll(() => quickChatTabReferences(restoredDialog), { timeout: 15_000 })
      .toEqual([terminalReference, firstReference, secondReference]);
    await expect(
      restoredDialog.locator(
        `[data-tab-reference="${firstReference}"] [data-testid="quick-chat-tab-name"]`,
      ),
    ).toHaveText(generatedTitle);
    await expect(
      restoredDialog.locator(
        `[data-tab-reference="${secondReference}"] [data-testid="quick-chat-tab-name"]`,
      ),
    ).toHaveText("Cache follow-up");
    await expect(restoredDialog.getByTestId("quick-chat-setup")).not.toBeVisible();

    await expect(restoredDialog.getByTestId("quick-terminal-tab")).toHaveCount(1);
  });
});
