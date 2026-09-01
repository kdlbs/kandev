import { type Locator, type Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { WsWatcher } from "../../helpers/causal-waits";

/**
 * Shared Quick Chat drivers. Kept out of the spec files so several specs
 * (single-client behaviour and cross-device sync) can drive the same surface
 * without duplicating its eager-init timing workarounds.
 */

const SETTLED_SESSION_STATES = new Set([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);
const ACTIVE_SESSION_STATES = new Set(["STARTING", "RUNNING"]);

export function quickChatTabReferences(dialog: Locator): Promise<string[]> {
  return dialog
    .getByTestId("quick-chat-sortable-tab")
    .evaluateAll((tabs) =>
      tabs
        .map((tab) => tab.getAttribute("data-tab-reference"))
        .filter((reference): reference is string => Boolean(reference)),
    );
}

/**
 * Establish a backend-confirmed settle point before arming waits for the next
 * turn. The WS watcher does not replay notifications, so the startup settle
 * must be observed through the session snapshot rather than a guessed delay.
 */
export async function waitForSessionSettledBaseline(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions.find((candidate) => candidate.id === sessionId);
        return session ? SETTLED_SESSION_STATES.has(session.state) : false;
      },
      {
        timeout: 30_000,
        message: `session ${sessionId} did not reach a settled baseline`,
      },
    )
    .toBe(true);
}

/**
 * Wait for the settle transition caused by the next turn. Requiring an active
 * old state prevents a queued startup/previous-turn notification from being
 * mistaken for the response to the message under test.
 */
export function waitForSessionSettled(ws: WsWatcher, sessionId: string) {
  return ws.waitForEvent("session.state_changed", {
    where: (payload) =>
      payload.session_id === sessionId &&
      ACTIVE_SESSION_STATES.has(payload.old_state ?? "") &&
      SETTLED_SESSION_STATES.has(payload.new_state ?? ""),
  });
}

export async function openQuickChatSetup(page: Page, navigateHome = true): Promise<Locator> {
  if (navigateHome) {
    await page.goto("/");
    await page.waitForLoadState("networkidle");
  }

  if (navigateHome) {
    // Open Quick Chat via keyboard shortcut (Cmd+Shift+Q / Ctrl+Shift+Q).
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${modifier}+Shift+q`);
  } else {
    await page.getByTestId("sidebar-quick-chat-shortcut").click();
  }

  // Wait for Quick Chat dialog.
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });

  // If a stale session tab is showing, click "+" to start a fresh setup form.
  const setup = dialog.getByTestId("quick-chat-setup");
  if (!(await setup.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await page.getByTestId("quick-chat-new-agent").click();
  }
  await expect(setup).toBeVisible({ timeout: 5_000 });
  return dialog;
}

export async function selectAgentIfNeeded(dialog: Locator, page: Page) {
  const selector = dialog.getByTestId("agent-profile-selector");
  await expect(selector).toBeVisible({ timeout: 10_000 });
  await expect(async () => {
    const text = await selector.innerText();
    if (!text.includes("Select agent")) return;

    await selector.click();
    const option = page.getByRole("option").first();
    await expect(option).toBeVisible({ timeout: 2_000 });
    await option.click();
    await expect(selector).not.toContainText("Select agent", { timeout: 2_000 });
  }).toPass({ timeout: 10_000, intervals: [250, 500, 1_000] });
}

export async function startQuickChatFromSetup(dialog: Locator, page: Page) {
  await selectAgentIfNeeded(dialog, page);
  await expect(dialog.getByTestId("quick-chat-start")).toBeEnabled({ timeout: 10_000 });
  await dialog.getByTestId("quick-chat-start").click();

  // Wait for chat input to appear AND become editable. Eager init means the
  // agent starts during the HTTP request, so the input is briefly disabled
  // while the FE store catches up to the RUNNING session state.
  const editor = dialog.locator(".tiptap.ProseMirror");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
}

export async function openQuickChatWithAgent(page: Page, navigateHome = true): Promise<Locator> {
  const dialog = await openQuickChatSetup(page, navigateHome);
  await startQuickChatFromSetup(dialog, page);
  return dialog;
}

export async function sendQuickChatMessage(dialog: Locator, page: Page, text: string) {
  const editor = dialog.locator(".tiptap.ProseMirror");
  // With eager init, the agent boots during picker -> tab transition and the
  // input can briefly toggle disabled while the FE store catches up. The whole
  // fill + submit must retry as one unit: after a reopen the conversation can
  // still be loading, so a fill can land on an editor that remounts (wiping
  // the text) before the submit click. A successful send clears the editor
  // synchronously, so each iteration either sends once and exits or sends
  // nothing and retries.
  await expect(async () => {
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 1_000 });
    await editor.click({ timeout: 1_000 });
    await editor.fill(text, { timeout: 1_000 });
    await expect(editor).toHaveText(text, { timeout: 1_000 });
    await dialog.getByTestId("submit-message-button").click({ timeout: 1_000 });
    await expect(editor).toHaveText("", { timeout: 2_000 });
  }).toPass({ timeout: 30_000, intervals: [250, 500, 1_000] });
}

export async function readQuickChatViewportLayout(dialog: Locator) {
  return dialog.evaluate(async (element) => {
    const animations = element.getAnimations({ subtree: true }).filter((animation) => {
      const iterations = animation.effect?.getComputedTiming().iterations;
      return typeof iterations === "number" && Number.isFinite(iterations);
    });
    await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));

    const content = element.querySelector<HTMLElement>('[data-testid="quick-chat-content"]');
    const messages = element.querySelector<HTMLElement>('[data-testid="quick-chat-messages"]');
    const composer = element.querySelector<HTMLElement>('[data-testid="chat-input-area"]');
    const messageScroller = messages?.querySelector<HTMLElement>(
      ".session-panel-content-wrapper > div",
    );
    if (!content || !composer || !messageScroller) {
      throw new Error("Quick Chat viewport layout elements are unavailable");
    }

    const dialogBounds = element.getBoundingClientRect();
    const contentBounds = content.getBoundingClientRect();
    const composerBounds = composer.getBoundingClientRect();
    return {
      dialogTop: dialogBounds.top,
      dialogBottom: dialogBounds.bottom,
      dialogClientHeight: element.clientHeight,
      dialogScrollHeight: element.scrollHeight,
      contentTop: contentBounds.top,
      contentBottom: contentBounds.bottom,
      composerTop: composerBounds.top,
      composerBottom: composerBounds.bottom,
      messageScrollerClientHeight: messageScroller.clientHeight,
      messageScrollerScrollHeight: messageScroller.scrollHeight,
    };
  });
}
