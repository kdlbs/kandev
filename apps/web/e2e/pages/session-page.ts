import { type Locator, type Page, expect } from "@playwright/test";
import { FileTreePage } from "./file-tree-page";
import { NewSessionDialogPage } from "./new-session-dialog-page";
import { dwell } from "../helpers/causal-waits";

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** Maps state-section labels to the per-task state icon data-testid. */
function sectionLabelToStateTestId(label: string): string {
  if (label === "Running") return "task-state-running";
  if (label === "Turn Finished") return "task-state-turn-finished";
  return "task-state-backlog";
}

const TERMINAL_READY_TIMEOUT = 30_000;

export class SessionPage {
  readonly chat: Locator;
  readonly sidebar: Locator;
  readonly terminal: Locator;
  readonly files: Locator;
  readonly changes: Locator;
  readonly planPanel: Locator;
  readonly stepper: Locator;
  readonly passthroughTerminal: Locator;
  readonly fileTree: FileTreePage;
  readonly newSessionDialogPage: NewSessionDialogPage;

  constructor(private readonly page: Page) {
    this.chat = page.getByTestId("session-chat");
    this.sidebar = page.getByTestId("task-sidebar");
    this.terminal = page.getByTestId("terminal-panel");
    this.files = page.getByTestId("files-panel");
    this.changes = page.getByTestId("changes-panel");
    this.planPanel = page.getByTestId("plan-panel");
    this.stepper = page.getByTestId("workflow-stepper");
    this.passthroughTerminal = page.getByTestId("passthrough-terminal");
    this.fileTree = new FileTreePage(page, this.files, () => this.activeChat());
    this.newSessionDialogPage = new NewSessionDialogPage(page);
  }

  // Port forward dialog locators
  get portForwardButton() {
    return this.page.getByTestId("port-forward-button");
  }
  get portForwardingMenuItem() {
    return this.page.getByTestId("port-forwarding-menu-item");
  }
  get mobileSessionMenu() {
    return this.page.getByTestId("mobile-session-menu");
  }
  get mobilePortForwardingToggle() {
    return this.page.getByTestId("mobile-port-forwarding-toggle");
  }
  get portForwardDialog() {
    return this.page.getByTestId("port-forward-dialog");
  }
  get portForwardRefresh() {
    return this.page.getByTestId("port-forward-refresh");
  }
  get portForwardInput() {
    return this.page.getByTestId("port-forward-port-input");
  }
  get portForwardAddButton() {
    return this.page.getByTestId("port-forward-add-button");
  }
  portForwardRow(port: number) {
    return this.page.getByTestId(`port-forward-row-${port}`);
  }
  portForwardTunnelToggle(port: number) {
    return this.portForwardRow(port).getByRole("button").first();
  }
  portForwardTunnelStart(port: number) {
    return this.portForwardRow(port).getByRole("button", { name: "Start", exact: true });
  }
  portForwardOpenBrowser(port: number) {
    return this.portForwardRow(port).getByTestId(`port-forward-open-browser-${port}`);
  }
  get browserPanel() {
    return this.page.locator('[data-testid="browser-panel"]:visible').first();
  }
  get browserAddressInput() {
    return this.browserPanel.locator("input").first();
  }

  async togglePortForwardingPreference(): Promise<void> {
    await this.addPanelButton().click();
    await expect(this.portForwardingMenuItem).toBeVisible();
    // The menu item is rendered before the session's agentctl launcher is
    // ready, but it is disabled until port forwarding can actually work.
    // Waiting for enabled avoids force-clicking a no-op during that startup
    // window, which otherwise leaves the top-bar control absent.
    await expect(this.portForwardingMenuItem).toBeEnabled({ timeout: 30_000 });
    const enabling = (await this.portForwardingMenuItem.getAttribute("aria-checked")) !== "true";
    await this.portForwardingMenuItem.click({ force: true });
    if (enabling) {
      await this.portForwardDialog
        .waitFor({ state: "visible", timeout: 5_000 })
        .catch(() => undefined);
    }
    if (await this.portForwardDialog.isVisible()) {
      await this.portForwardDialog.getByRole("button", { name: "Close" }).click();
    }
    await this.page.keyboard.press("Escape");
    await expect(this.portForwardingMenuItem).toBeHidden();
    await expect(this.portForwardDialog).toBeHidden();
  }

  async enablePortForwarding(): Promise<void> {
    if (await this.portForwardButton.isVisible()) return;
    await this.togglePortForwardingPreference();
    await expect(this.portForwardButton).toBeVisible();
  }

  // Chat status bar locators
  appStatusBar() {
    return this.page.getByTestId("app-status-bar");
  }
  chatStatusBar() {
    return this.page.getByTestId("chat-status-bar");
  }
  prMergedBanner() {
    return this.page.getByTestId("pr-merged-banner");
  }
  prMergedArchiveButton() {
    return this.page.getByTestId("pr-merged-archive-button");
  }
  prMergedArchiveConfirmButton() {
    return this.page.getByTestId("pr-merged-archive-confirm");
  }
  prMergedDismissButton() {
    return this.page.getByTestId("pr-merged-dismiss-button");
  }
  prClosedBanner() {
    return this.page.getByTestId("pr-closed-banner");
  }
  prClosedArchiveButton() {
    return this.page.getByTestId("pr-closed-archive-button");
  }
  prClosedArchiveConfirmButton() {
    return this.page.getByTestId("pr-closed-archive-confirm");
  }
  prClosedDismissButton() {
    return this.page.getByTestId("pr-closed-dismiss-button");
  }
  prStatusChip() {
    return this.activeChat().getByTestId("chat-status-bar").getByTestId("pr-status-chip");
  }
  todoIndicator() {
    return this.activeChat().getByTestId("todo-indicator");
  }
  /** Span wrapper around the resume button — used to trigger tooltip on disabled state. */
  failedSessionResumeWrapper(): Locator {
    return this.page.getByTestId("failed-session-resume-wrapper");
  }
  /** Cancel button shown in the chat toolbar while an agent turn is running. */
  cancelAgentButton(): Locator {
    return this.page.getByTestId("cancel-agent-button");
  }
  /** The currently visible chat panel when dockview keeps background panels mounted. */
  activeChat(): Locator {
    return this.page.locator("[data-testid='session-chat']:visible").first();
  }

  /**
   * Wait for the session chat panel to be visible.
   *
   * When multiple session tabs are open, multiple session-chat panels exist in
   * the DOM but only the active one is visible. Use :visible to avoid matching
   * a hidden background panel (which would cause the wait to time out).
   *
   * Under CI shard load the freshly-navigated task page can be slow to hydrate:
   * the SSR boot payload + React mount + WS connect sequence races, and a single
   * hard `waitFor` occasionally exceeds its budget before the chat panel mounts.
   * Reloading re-drives SSR hydration and reliably recovers, so instead of one
   * fixed wait we poll with a bounded reload-and-retry loop (same recovery shape
   * as `waitForChatIdle`). The fast path stays instant when the chat is already
   * visible.
   */
  async waitForLoad(timeout = 15_000) {
    const chat = this.activeChat();
    // Fast path: already foregrounded (common case, no reload cost).
    if (await chat.isVisible()) return;

    const attemptTimeout = Math.min(timeout, Math.max(5_000, Math.floor(timeout / 2)));
    const start = Date.now();
    let lastReloadAt = start;

    while (Date.now() - start < timeout) {
      const remaining = timeout - (Date.now() - start);
      const now = Date.now();
      // Re-drive SSR hydration once per attemptTimeout slice while budget remains
      // for the reloaded page to settle.
      if (now - lastReloadAt >= attemptTimeout && remaining > attemptTimeout) {
        lastReloadAt = now;
        await this.page.reload();
      }
      await chat
        .waitFor({ state: "visible", timeout: Math.min(attemptTimeout, remaining) })
        .catch(() => undefined);
      if (await chat.isVisible()) return;
    }

    // Final bounded check: still throws on a genuinely stuck page.
    await chat.waitFor({ state: "visible", timeout: attemptTimeout });
  }

  /**
   * Foreground the session chat and wait for it to be visible.
   *
   * After the unified AppSidebar overhaul, switching tasks via the sidebar
   * restores each task's saved dockview env layout. That restored layout can
   * land the chat panel as a *non-active* background tab in the right-column
   * group (e.g. behind Files/Changes), so the chat is mounted but not visible
   * and a plain `waitForLoad()` (which gates on `session-chat:visible`) times
   * out. Clicking the session tab brings the chat to the foreground — exactly
   * what a user does to read the conversation after switching tasks — and then
   * we wait for the now-visible chat. No-op (still waits) when the chat is
   * already foregrounded.
   */
  async showSessionContext(timeout = 15_000): Promise<void> {
    const tab = this.page.locator("[data-testid^='session-tab-']:visible").first();
    await tab.waitFor({ state: "visible", timeout });
    // Clicking a tab that's already active is harmless; clicking a background
    // one promotes its panel to the foreground.
    await tab.click();
    await this.activeChat().waitFor({ state: "visible", timeout });
  }

  /**
   * Wait for the chat to be idle (input placeholder visible, agent not busy).
   *
   * A fresh task can still miss the first persisted session-state transition
   * while the page hydrates under CI load. Re-drive SSR hydration periodically
   * so a stale busy state does not consume the whole idle wait budget.
   *
   * After a backend restart, auto-resume can briefly surface the recovery
   * prompt ("Environment setup failed"); click through it when visible.
   */
  async waitForChatIdle(opts: { timeout?: number; requireEditable?: boolean } = {}) {
    const softTotalTimeout = opts.timeout ?? 45_000;
    const attemptTimeout = Math.min(15_000, Math.max(5_000, Math.floor(softTotalTimeout / 3)));
    const pollSlice = 1_500;
    const idle = this.anyIdleInput();
    const editor = this.activeChat().locator(".tiptap.ProseMirror:visible").first();
    const isReady = async () =>
      (await idle.isVisible()) && (!opts.requireEditable || (await editor.isEditable()));
    const start = Date.now();
    let lastReloadAt = start;

    while (Date.now() - start < softTotalTimeout) {
      if (await isReady()) return;

      const resumeButton = this.recoveryResumeButton();
      if (await resumeButton.isVisible()) {
        if (await resumeButton.isEnabled()) {
          await resumeButton.click();
        }
        await resumeButton.waitFor({ state: "hidden", timeout: pollSlice }).catch(() => undefined);
        continue;
      }

      const now = Date.now();
      const remaining = Math.max(1, softTotalTimeout - (now - start));
      if (now - lastReloadAt >= attemptTimeout && remaining > pollSlice) {
        lastReloadAt = now;
        await this.page.reload();
        await this.activeChat()
          .waitFor({ state: "visible", timeout: Math.min(attemptTimeout, remaining) })
          .catch(() => undefined);
        continue;
      }

      const timeout = Math.min(pollSlice, remaining);
      if (opts.requireEditable) {
        await expect
          .poll(isReady, { timeout })
          .toBe(true)
          .catch(() => undefined);
      } else {
        await idle.waitFor({ state: "visible", timeout }).catch(() => undefined);
      }
    }

    // Final bounded check: still throws on a genuinely stuck session, but gives
    // the last hydration attempt a full attemptTimeout slice to land.
    if (opts.requireEditable) {
      await expect.poll(isReady, { timeout: attemptTimeout }).toBe(true);
      return;
    }
    await idle.waitFor({ state: "visible", timeout: attemptTimeout });
  }

  /** Wait for the passthrough terminal to be visible (for TUI/passthrough sessions). */
  async waitForPassthroughLoad(timeout = 15_000) {
    await this.passthroughTerminal.waitFor({ state: "visible", timeout });
  }

  /** Wait for the passthrough loading indicator to be visible (scoped to agent terminal). */
  async waitForPassthroughLoading(timeout = 5_000) {
    await this.passthroughTerminal
      .getByTestId("passthrough-loading")
      .waitFor({ state: "visible", timeout });
  }

  /** Wait for the passthrough loading indicator to disappear (scoped to agent terminal). */
  async waitForPassthroughLoaded(timeout = 15_000) {
    await this.passthroughTerminal
      .getByTestId("passthrough-loading")
      .waitFor({ state: "hidden", timeout });
  }

  /**
   * Return the foreground panel for a test id.
   *
   * Dockview keeps background task panels mounted while switching tasks. A
   * page-level DOM query can therefore read a stale terminal buffer even
   * though the visible panel has already connected.
   */
  private activePanel(testId: string): Locator {
    return this.page.locator(`[data-testid="${testId}"]:visible`).first();
  }

  /**
   * Read the text content of an xterm.js terminal buffer.
   * xterm renders to canvas/WebGL so text isn't in the DOM. Uses the
   * __xtermReadBuffer() helper exposed on the terminal container element.
   */
  private async readXtermBuffer(testId: string): Promise<string> {
    const panel = this.activePanel(testId);
    if ((await panel.count()) === 0) return "";

    return panel.evaluate((panelElement) => {
      const xtermEl = panelElement.querySelector(".xterm");
      type XC = HTMLElement & { __xtermReadBuffer?: () => string };
      const container = xtermEl?.parentElement as XC | null | undefined;
      return container?.__xtermReadBuffer?.() ?? "";
    });
  }

  /**
   * Assert the passthrough terminal buffer contains the given text.
   */
  async expectPassthroughHasText(text: string, timeout = 15_000): Promise<void> {
    await expect
      .poll(async () => (await this.readXtermBuffer("passthrough-terminal")).includes(text), {
        timeout,
        message: `Expected passthrough terminal to contain "${text}"`,
      })
      .toBe(true);
  }

  /**
   * Assert the passthrough terminal buffer does NOT contain the given text.
   * Waits briefly to confirm absence (text could arrive asynchronously).
   */
  async expectPassthroughNotHasText(text: string, stableMs = 2_000): Promise<void> {
    const start = Date.now();
    while (Date.now() - start < stableMs) {
      if ((await this.readXtermBuffer("passthrough-terminal")).includes(text)) {
        throw new Error(`Expected passthrough terminal NOT to contain "${text}", but it was found`);
      }
      await dwell(
        this.page,
        200,
        "poll-interval",
        "sampling interval for the stability window above; the assertion is that the text never appears, so the loop keeps re-reading the buffer across real elapsed time",
      );
    }
  }

  /** Scoped to the sidebar — finds task title text rendered by TaskItem. */
  taskInSidebar(title: string): Locator {
    return this.sidebar.getByText(title, { exact: false });
  }

  sidebarTaskItem(title: string): Locator {
    return this.sidebar.getByTestId("sidebar-task-item").filter({
      has: this.page.getByText(title, { exact: false }),
    });
  }

  activeSidebarTaskItem(title: string): Locator {
    return this.sidebarTaskItem(title).and(this.sidebar.locator('[aria-current="true"]'));
  }

  async openSidebarTaskContextMenu(title: string): Promise<void> {
    const taskRow = this.sidebarTaskItem(title).first();
    await taskRow.waitFor({ state: "visible" });
    await taskRow.click({ button: "right" });
  }

  async openCreateSubtaskForSidebarTask(title: string): Promise<void> {
    await this.openSidebarMenuAndClick(title, "Create Subtask");
  }

  async sendSidebarTaskToWorkflow(
    title: string,
    workflowId: string,
    stepId: string,
  ): Promise<void> {
    await this.openSidebarTaskContextMenu(title);
    await this.page.getByTestId("task-context-send-to-workflow").hover();
    await this.page.getByTestId(`task-context-workflow-${workflowId}`).hover();
    await this.page.getByTestId(`task-context-step-${stepId}`).click();
  }

  /**
   * Sidebar state indicator — returns the first icon matching the given state label.
   * Accepts "Turn Finished" (review/completed), "Running" (in-progress), or "Backlog".
   */
  sidebarSection(label: string): Locator {
    if (label === "Turn Finished") {
      return this.sidebar
        .locator(
          '[data-testid="task-state-turn-finished"], [data-testid="task-state-workflow-complete"]',
        )
        .first();
    }
    const testId = sectionLabelToStateTestId(label);
    return this.sidebar.getByTestId(testId).first();
  }

  /**
   * Task item in the sidebar matching both a title and a state label.
   * Accepts "Turn Finished" (review/completed), "Running" (in-progress), or "Backlog".
   */
  taskInSection(title: string, sectionLabel: string): Locator {
    if (sectionLabel === "Turn Finished") {
      return this.sidebar
        .getByTestId("sidebar-task-item")
        .filter({ has: this.page.getByText(title, { exact: false }) })
        .filter({
          has: this.page.locator(
            '[data-testid="task-state-turn-finished"], [data-testid="task-state-workflow-complete"]',
          ),
        });
    }
    const testId = sectionLabelToStateTestId(sectionLabel);
    return this.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ has: this.page.getByText(title, { exact: false }) })
      .filter({ has: this.page.getByTestId(testId) });
  }

  /** Foreground or detached-background working status indicator. */
  agentStatus(): Locator {
    return this.page.getByRole("status", {
      name: /Agent is (starting|running)|Background work is running/,
    });
  }

  /** Divider that appears after the "New session started" status message is rendered. */
  turnComplete(): Locator {
    return this.page.getByTestId("agent-turn-complete");
  }

  /** Chat input placeholder when agent is idle (default mode). */
  idleInput(): Locator {
    return this.activeChat().locator('[data-placeholder="Continue working on the task..."]');
  }

  /** Chat input placeholder when agent is idle in any current mode. */
  anyIdleInput(): Locator {
    return this.activeChat()
      .locator('[data-placeholder="Continue working on the task..."]')
      .or(this.activeChat().locator('[data-placeholder="Continue working on the plan..."]'))
      .or(this.activeChat().locator('[data-placeholder="Continue working on the file..."]'));
  }

  /** Chat input placeholder when agent is idle (plan mode). */
  planModeInput(): Locator {
    return this.activeChat().locator('[data-placeholder="Continue working on the plan..."]');
  }

  /**
   * "Plan mode" badge shown on a message that was sent with plan mode active.
   * Appears when message.metadata.plan_mode = true, which the backend sets when
   * a session is auto-started via the enable_plan_mode workflow event.
   */
  planModeBadge(): Locator {
    return this.chat.getByText("Plan mode", { exact: true });
  }

  /** Clarification overlay (visible when a clarification request is pending). */
  clarificationOverlay(): Locator {
    return this.activeChat().getByTestId("clarification-overlay");
  }

  /**
   * The persistent bar wrapping the clarification overlay. Stays mounted
   * (collapsed to a header row) while the bundle is pending, even after the
   * user dismisses it with Escape or the collapse toggle.
   */
  clarificationBar(): Locator {
    return this.activeChat().getByTestId("clarification-overlay-container");
  }

  /** Expand/collapse toggle in the clarification bar's header row. */
  clarificationCollapseToggle(): Locator {
    // The expanded overlay stays mounted but is hidden when the compact bar
    // is shown, so scope this locator to the one visible toggle.
    return this.activeChat().locator('[data-testid="clarification-collapse-toggle"]:visible');
  }

  /** Shared context shown once above the active clarification question. */
  clarificationContext(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-context");
  }

  /** A specific clarification option button by its text label. */
  clarificationOption(text: string): Locator {
    return this.clarificationOverlay()
      .getByTestId("clarification-option")
      .filter({ hasText: text });
  }

  /** Skip (X) button on the clarification overlay. */
  clarificationSkip(): Locator {
    return this.page.getByTestId("clarification-skip");
  }

  /** Header status shown while a clarification answer is being submitted. */
  clarificationSubmittingStatus(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-submitting-status");
  }

  /** Custom text input on the clarification overlay. */
  clarificationInput(): Locator {
    return this.page.getByTestId("clarification-input");
  }

  /** Apparent custom-answer row surrounding the textarea. */
  clarificationCustomInput(): Locator {
    return this.activeChat().getByTestId("clarification-custom-input");
  }

  /** Inline Send button shown next to the custom input on touch devices. */
  clarificationCustomSubmit(): Locator {
    return this.page.getByTestId("clarification-custom-submit");
  }

  /** Deferred notice shown when agent has disconnected from clarification. */
  clarificationDeferredNotice(): Locator {
    return this.page.getByTestId("clarification-deferred-notice");
  }

  /** Expired notice rendered in chat history when the agent timed out waiting. */
  clarificationExpiredNotice(): Locator {
    return this.page.getByTestId("clarification-expired-notice");
  }

  /** Label span inside a clarification option. */
  clarificationOptionLabels(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-option-label");
  }

  /** Description span inside a clarification option (hidden when option has none). */
  clarificationOptionDescriptions(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-option-description");
  }

  /** All question cards rendered for the active clarification bundle. */
  clarificationQuestionCards(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-question-card");
  }

  /** A single question card by its question id (matches metadata.question_id). */
  clarificationQuestionCardById(questionId: string): Locator {
    return this.clarificationOverlay().locator(
      `[data-testid="clarification-question-card"][data-question-id="${questionId}"]`,
    );
  }

  /** Group-wide progress chip "N of M answered" — only shown for bundles >1. */
  clarificationGroupProgress(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-group-progress");
  }

  /** Per-question "Question N of M" progress chip. */
  clarificationProgressChips(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-progress-chip");
  }

  /** Custom text input within a specific question card. */
  clarificationInputForQuestion(questionId: string): Locator {
    return this.clarificationQuestionCardById(questionId).getByTestId("clarification-input");
  }

  /** Container around the custom text input — exposes data-active for selection state. */
  clarificationCustomInputContainerForQuestion(questionId: string): Locator {
    return this.clarificationQuestionCardById(questionId).getByTestId("clarification-custom-input");
  }

  /** Option button (by visible label text) inside a specific question card. */
  clarificationOptionForQuestion(questionId: string, text: string): Locator {
    return this.clarificationQuestionCardById(questionId)
      .getByTestId("clarification-option")
      .filter({ hasText: text });
  }

  /** All step buttons in the horizontal stepper. */
  clarificationSteps(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-step");
  }

  /** A single step in the stepper, by its 0-based index. */
  clarificationStep(index: number): Locator {
    return this.clarificationOverlay().locator(
      `[data-testid="clarification-step"][data-step-index="${index}"]`,
    );
  }

  /** Back button inside the carousel nav. */
  clarificationPrev(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-prev");
  }

  /** Next button inside the carousel nav. */
  clarificationNext(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-next");
  }

  /** Sticky "Submit" button in the overlay header (multi-question only). */
  clarificationSubmit(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-submit");
  }

  /** All visible "Approve / Deny" rows for pending permission requests. */
  permissionActionRows(): Locator {
    return this.chat.getByTestId("permission-action-row");
  }

  /** All "Approve" buttons for pending permission requests. */
  permissionApproveButtons(): Locator {
    return this.chat.getByTestId("permission-approve");
  }

  /** Kandev-MCP-only "Approve" buttons (excludes the generic ToolCallMessage
   *  fallback row that may briefly duplicate the same pending_id). */
  kandevPermissionApproveButtons(): Locator {
    return this.chat.getByTestId("kandev-tool-permission").getByTestId("permission-approve");
  }

  /** Reset context button in the chat input toolbar. */
  resetContextButton(): Locator {
    return this.page.getByTestId("reset-context-button");
  }

  /** Confirm button in the reset context alert dialog. */
  resetContextConfirm(): Locator {
    return this.page.getByTestId("reset-context-confirm");
  }

  /** "Resume session" button shown after agent crash. */
  recoveryResumeButton(): Locator {
    return this.page.getByTestId("recovery-resume-button");
  }

  /** Error returned by a manual session recovery action. */
  recoveryError(): Locator {
    return this.activeChat().getByTestId("session-recovery-error");
  }

  /** Explicit action for continuing a conversation on a replacement branch. */
  recoveryNewBranchButton(): Locator {
    return this.activeChat().getByTestId("recovery-new-branch-button");
  }

  /** Read-only workspace restore action shown after a recovery failure. */
  recoveryRestoreWorkspaceButton(): Locator {
    return this.activeChat().getByTestId("recovery-restore-workspace-button");
  }

  /** Non-blocking notice shown after automatic read-only workspace restore. */
  recoveryReadOnlyNotice(): Locator {
    return this.activeChat().getByTestId("session-recovery-notice");
  }

  /** Persisted warning shown after the original branch is replaced. */
  branchRecreatedWarning(): Locator {
    return this.activeChat().getByTestId("branch-recreated-warning");
  }

  /** "Start fresh session" button shown after agent crash. */
  recoveryFreshButton(): Locator {
    return this.page.getByTestId("recovery-fresh-button");
  }

  /** Terminal-state banner shown when the active session has completed. */
  completedSessionBanner(): Locator {
    return this.activeChat().getByTestId("completed-session-banner");
  }

  /** "New Agent" action shown for a completed session. */
  completedSessionNewAgentButton(): Locator {
    return this.completedSessionBanner().getByTestId("completed-session-new-agent-button");
  }

  /** "Cancel" button shown on the yellow transient-retry (529 Overloaded) card. */
  recoveryCancelRetryButton(): Locator {
    return this.page.getByTestId("recovery-cancel-retry-button");
  }

  /** The yellow provider-error retry status card. */
  transientRetryCard(): Locator {
    return this.activeChat().getByTestId("transient-retry-card");
  }

  /** Context reset divider shown in chat after resetting agent context. */
  contextResetDivider(): Locator {
    return this.chat.getByText("Context reset");
  }

  /**
   * Delete a task via the sidebar context menu.
   * Hovers to reveal the menu trigger, opens it, clicks "Delete",
   * and confirms the delete dialog.
   */
  async deleteTaskInSidebar(title: string): Promise<void> {
    await this.openSidebarMenuAndClick(title, "Delete");
    const confirmButton = this.page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Delete" });
    await confirmButton.click();
  }

  /**
   * Archive a task via the sidebar context menu.
   * Hovers to reveal the menu trigger, opens it, clicks "Archive",
   * and confirms the local archive surface or cascade dialog.
   */
  async archiveTaskInSidebar(title: string, options: { cascade?: boolean } = {}): Promise<void> {
    await this.openSidebarMenuAndClick(title, "Archive");
    if (options.cascade) {
      const cascadeCheckbox = this.page.getByTestId("archive-cascade-checkbox");
      await cascadeCheckbox.click();
    }
    await this.page.getByTestId("archive-task-confirm").click();
  }

  /**
   * Open a sidebar task's dropdown menu and click an item.
   * Retries the full open-click sequence if the menu gets detached by a
   * React re-render (e.g. WS-driven sidebar update) between open and click.
   */
  async openSidebarMenuAndClick(title: string, itemName: string, retries = 3): Promise<void> {
    const taskRow = this.sidebar.locator('[role="button"]').filter({ hasText: title });
    for (let attempt = 0; attempt < retries; attempt++) {
      try {
        await taskRow.hover();
        await taskRow.getByRole("button", { name: "Task actions" }).click();
        const menuItem = this.page.getByRole("menuitem", { name: itemName });
        await menuItem.waitFor({ state: "visible", timeout: 3_000 });
        await menuItem.click({ timeout: 3_000 });
        return;
      } catch {
        // Menu was likely detached by a re-render — dismiss and retry
        await this.page.keyboard.press("Escape");
        await dwell(
          this.page,
          500,
          "unverified",
          "spacing before the next attempt in this menu-retry loop; the menu was detached mid-render and nothing was identified that signals it is safe to re-open",
        );
      }
    }
    // Final attempt without catch
    await taskRow.hover();
    await taskRow.getByRole("button", { name: "Task actions" }).click();
    await this.page.getByRole("menuitem", { name: itemName }).click();
  }

  stepperStep(name: string): Locator {
    return this.page.locator(`[data-testid="workflow-step-${name}"][aria-current="step"]`);
  }

  /** PR button in the topbar (visible only when a PR is associated). */
  prTopbarButton(): Locator {
    return this.page.getByTestId("pr-topbar-button");
  }

  /** PR detail panel content when a linked GitHub pull request is selected. */
  prDetailPanel(): Locator {
    return this.page.getByTestId("pr-detail-panel");
  }

  /** "Approve PR" button inside the PR detail panel header. Hidden when the
   * current GitHub user authored the PR (self-approval is rejected upstream). */
  prApproveButton(): Locator {
    return this.page.getByTestId("pr-approve-button");
  }

  /** Submitted review row scoped by its normalized GitHub author login. */
  prSubmittedReview(author: string): Locator {
    return this.page.getByTestId(`change-request-submitted-review-${author.trim().toLowerCase()}`);
  }

  /** Pending reviewer row scoped by its normalized GitHub author login. */
  prPendingReviewer(author: string): Locator {
    return this.page.getByTestId(`change-request-pending-reviewer-${author.trim().toLowerCase()}`);
  }

  /** Re-request action scoped by its normalized GitHub author login. */
  prReRequestReviewButton(author: string): Locator {
    return this.page.getByTestId(
      `change-request-review-action-rerequest-review-${author.trim().toLowerCase()}`,
    );
  }

  // --- PR CI accessors: desktop hover popover + chip + mobile chip drawer ---

  /** The single-PR hover popover content (visible after hovering the topbar button). */
  prTopbarPopover(): Locator {
    return this.page.getByTestId("pr-topbar-popover");
  }

  /** Compact PR/CI status chip rendered in the chat status bar. */
  prStatusChip(): Locator {
    return this.activeChat().getByTestId("chat-status-bar").getByTestId("pr-status-chip");
  }

  /** Mobile bottom-sheet drawer that hosts the PR CI popover. */
  prStatusChipDrawer(): Locator {
    return this.page.getByTestId("pr-status-chip-drawer");
  }

  /** Close button inside the chip's mobile drawer. */
  prStatusChipDrawerClose(): Locator {
    return this.page.getByTestId("pr-status-chip-drawer-close");
  }

  /** PRCIPopover body when rendered inside the mobile chip drawer. */
  prStatusChipPopoverInner(): Locator {
    return this.prStatusChipDrawer().getByTestId("pr-topbar-popover-inner");
  }

  /** Tap the chip and wait for the mobile drawer to be visible. */
  async tapPRStatusChip(): Promise<void> {
    await this.prStatusChip().tap();
    await expect(this.prStatusChipDrawer()).toBeVisible({ timeout: 5_000 });
  }

  // --- GitLab MR status chip accessors: mirrors the PR status chip shape
  // above, including its scoping (spec: gitlab-mr-status-chip, Constraints).

  /** Compact GitLab MR status chip rendered in the chat status bar. */
  mrStatusChip(): Locator {
    return this.activeChat().getByTestId("chat-status-bar").getByTestId("mr-status-chip");
  }

  /** Compact GitLab MR status chip rendered in the passthrough toolbar's status row. */
  mrStatusChipInPassthrough(): Locator {
    return this.page.getByTestId("passthrough-status-row").getByTestId("mr-status-chip");
  }

  /** Mobile bottom-sheet drawer that hosts the chip's MRCIPopover body. */
  mrStatusChipDrawer(): Locator {
    return this.page.getByTestId("mr-status-chip-drawer");
  }

  /** Close button inside the chip's mobile drawer. */
  mrStatusChipDrawerClose(): Locator {
    return this.page.getByTestId("mr-status-chip-drawer-close");
  }

  /**
   * MRCIPopover body when rendered inside the chip's own disclosure — the
   * hover popover on a fine pointer, or the drawer on a coarse pointer.
   * `mr-topbar-popover-inner` is also emitted by MRTopbarButton's own
   * popover on the same route, so this scopes through the chip's own
   * wrapper testid (`mr-status-chip-popover` / `mr-status-chip-drawer`)
   * rather than resolving the inner testid globally.
   */
  mrStatusChipPopoverInner(): Locator {
    return this.page
      .getByTestId("mr-status-chip-popover")
      .getByTestId("mr-topbar-popover-inner")
      .or(this.mrStatusChipDrawer().getByTestId("mr-topbar-popover-inner"));
  }

  /** Tap the chip and wait for the mobile drawer to be visible. */
  async tapMRStatusChip(): Promise<void> {
    await this.mrStatusChip().tap();
    await expect(this.mrStatusChipDrawer()).toBeVisible({ timeout: 5_000 });
  }

  /** Multi-PR aggregate popover content (segmented tabs + selected PR's CI). */
  prTopbarPopoverAggregate(): Locator {
    return this.page.getByTestId("pr-multi-popover");
  }

  /** A single PR tab inside the multi-PR aggregate popover, by owner + repo + PR number. */
  prMultiPopoverTab(owner: string, repo: string, prNumber: number): Locator {
    return this.page.getByTestId(`pr-popover-tab-${owner}-${repo}-${prNumber}`);
  }

  /** Unlink control for one PR association in the multi-PR popover. */
  prMultiPopoverRemove(owner: string, repo: string, prNumber: number): Locator {
    const activePopover = this.page.locator("[data-testid='pr-multi-popover']:visible").last();
    return activePopover.getByTestId(`pr-popover-remove-${owner}-${repo}-${prNumber}`);
  }

  /**
   * A specific bucket group inside the popover by kind.
   *
   * Scoped to the TOPBAR popover (`pr-topbar-popover`) — the chip's HoverCard
   * renders the same inner content without that wrapper, so specs asserting
   * check groups after hovering the status chip need a chip-scoped variant.
   */
  prCheckGroup(kind: "passed" | "in_progress" | "failed"): Locator {
    return this.prTopbarPopover().locator(`[data-testid='pr-check-group'][data-kind='${kind}']`);
  }

  /** Count number rendered inside a bucket group's header. */
  prCheckGroupCount(kind: "passed" | "in_progress" | "failed"): Locator {
    return this.prCheckGroup(kind).getByTestId("pr-check-group-count");
  }

  /** A workflow row by its workflow name (the part before " / " in CheckRun.name). */
  prWorkflowRow(workflow: string): Locator {
    return this.prTopbarPopover().locator(
      `[data-testid='pr-workflow-row'][data-workflow='${workflow}']`,
    );
  }

  /** Open-on-GitHub button inside a workflow row. */
  prWorkflowOpenButton(workflow: string): Locator {
    return this.prWorkflowRow(workflow).getByTestId("pr-workflow-open");
  }

  /** "+ ctx" button inside a (failed) workflow row. */
  prWorkflowAddContextButton(workflow: string): Locator {
    return this.prWorkflowRow(workflow).getByTestId("pr-workflow-add-context");
  }

  /** Review state line ("Approved 1 / 2 required" etc.). */
  prReviewRow(): Locator {
    return this.prTopbarPopover().getByTestId("pr-review-row");
  }

  /** Unresolved-comments row inside the popover. */
  prCommentsRow(): Locator {
    return this.prTopbarPopover().getByTestId("pr-comments-row");
  }

  /** Header PR-link icon (top-right corner of the popover). */
  prPopoverPRLink(): Locator {
    return this.prTopbarPopover().getByTestId("pr-popover-pr-link");
  }

  /** Header external-link icon (top-right corner of the popover). */
  prPopoverExternalLink(): Locator {
    return this.prTopbarPopover().getByTestId("pr-popover-external-link");
  }

  /** Footer "updated Ns ago" timestamp text. */
  prPopoverUpdatedAt(): Locator {
    return this.prTopbarPopover().getByTestId("pr-popover-updated-at");
  }

  /** Footer spinner + "Updating…", shown while a refresh is in flight. */
  prPopoverUpdating(): Locator {
    return this.prTopbarPopover().getByTestId("pr-popover-updating");
  }

  /** Empty-state row when the PR has no checks yet. */
  prChecksEmpty(): Locator {
    return this.prTopbarPopover().getByTestId("pr-checks-empty");
  }

  /** "Reconnect GitHub" link rendered when auth health is unhealthy. */
  prPopoverReconnectLink(): Locator {
    return this.prTopbarPopover().getByTestId("pr-popover-reconnect-link");
  }

  /**
   * Open the popover by hovering the topbar button. Waits for the open delay
   * (~150ms in PRTopbarButton) plus a small buffer.
   *
   * To keep the popover open while interacting with rows, the test should
   * hover the popover content directly afterwards (Playwright hover() over a
   * row inside the popover keeps the cursor in the open region).
   */
  async hoverPRTopbar(): Promise<void> {
    await expect(async () => {
      const button = this.prTopbarButton();
      await button.scrollIntoViewIfNeeded();
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      await button.focus();
      await this.page.mouse.move(0, 0);
      await this.page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
      await button.dispatchEvent("mouseover", { bubbles: true });
      await button.dispatchEvent("mouseenter", { bubbles: false });
      await button.dispatchEvent("mousemove", { bubbles: true });
      await expect(this.prTopbarPopover()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 10_000 });
  }

  /**
   * Desktop chip hover popover content. The chip's Popover renders PRCIPopover
   * (test id `pr-topbar-popover-inner`) directly, without the topbar's
   * `pr-topbar-popover` wrapper, so this is the chip-scoped accessor for the
   * open hover card.
   */
  prChipPopover(): Locator {
    // Scope to the visible instance: dock/mobile layouts can leave stale or
    // hidden popover mounts in the DOM, and an unscoped getByTestId would bind
    // to one of those and make hover assertions flaky.
    return this.page.locator("[data-testid='pr-topbar-popover-inner']:visible").first();
  }

  /**
   * Open the chip's hover popover by hovering the chat-status-bar CI chip.
   * Mirrors {@link hoverPRTopbar}: moves the real cursor onto the chip and
   * also dispatches the hover events so the open is reliable across browsers.
   */
  async hoverPRChip(): Promise<void> {
    await expect(async () => {
      const chip = this.prStatusChip();
      await chip.scrollIntoViewIfNeeded();
      const box = await chip.boundingBox();
      expect(box).not.toBeNull();
      await this.page.mouse.move(0, 0);
      await this.page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
      await chip.dispatchEvent("mouseover", { bubbles: true });
      await chip.dispatchEvent("mouseenter", { bubbles: false });
      await chip.dispatchEvent("mousemove", { bubbles: true });
      await expect(this.prChipPopover()).toBeVisible({ timeout: 1_500 });
    }).toPass({ timeout: 10_000 });
  }

  /** Dockview tab for canonical or keyed PR detail content. */
  prDetailTab(): Locator {
    return this.page
      .locator(".dv-default-tab")
      .filter({ hasText: /^(PR Details|Pull Request|PR #\d+)$/ });
  }

  /** Click a dockview tab by its visible label (e.g. "Changes", "Files", "Terminal"). */
  async clickTab(label: string, options: { force?: boolean } = {}): Promise<void> {
    const tab = this.page
      .locator(".dv-default-tab:visible")
      .filter({ hasText: new RegExp(`^${escapeRegExp(label)}(?: \\(\\d+\\))?$`) })
      .first();
    await expect(tab).toBeVisible();
    await tab.click(options);
  }

  /**
   * Click the session/chat tab regardless of its current title.
   * Session tabs are renamed from "Agent" to "#N AgentName" by useChatSessionTitle,
   * so this uses the stable data-testid on the ContextMenuTrigger instead.
   */
  async clickSessionChatTab(): Promise<void> {
    await this.page.locator('[data-testid^="session-tab-"]:visible').first().click();
  }

  /** Main Changes-panel button that asks the agent to create a walkthrough. */
  changesRequestWalkthroughButton(): Locator {
    return this.changes.getByTestId("changes-request-walkthrough");
  }

  /** Compact request button in the expanded Review Changes toolbar. */
  reviewRequestWalkthroughButton(): Locator {
    return this.page.getByTestId("review-request-walkthrough");
  }

  /** Expanded Review dialog shared by the desktop and mobile task layouts. */
  reviewDialog(): Locator {
    return this.page.getByRole("dialog", { name: "Review Changes" });
  }

  /** Current-PR trigger rendered in Review when the task has multiple PRs. */
  reviewPRSelectorTrigger(): Locator {
    return this.page.getByTestId("review-pr-selector-trigger");
  }

  /** Portaled PR selector menu; intentionally page-scoped rather than dialog-scoped. */
  reviewPRSelectorMenu(): Locator {
    return this.page.getByTestId("review-pr-selector-menu");
  }

  /** One PR choice in the expanded Review selector. */
  reviewPRSelectorItem(owner: string, repo: string, prNumber: number): Locator {
    return this.page.getByTestId(`review-pr-selector-item-${owner}-${repo}-${prNumber}`);
  }

  /** Sticky diff header for one file in the expanded Review dialog. */
  reviewFileHeader(path: string): Locator {
    return this.reviewDialog().locator(
      `[data-testid="review-file-header"][data-file-path=${JSON.stringify(path)}]`,
    );
  }

  /**
   * Read visible Review diff text from @pierre/diffs shadow roots.
   *
   * Dockview can leave hidden diff surfaces mounted, so scope to the active
   * Review dialog and ignore zero-size/hidden containers before reading them.
   */
  async reviewDiffText(): Promise<string> {
    return this.reviewDialog().evaluate((dialog) => {
      const visibleContainers = Array.from(dialog.querySelectorAll("diffs-container")).filter(
        (container) => {
          const bounds = container.getBoundingClientRect();
          const style = window.getComputedStyle(container);
          return (
            bounds.width > 0 &&
            bounds.height > 0 &&
            style.display !== "none" &&
            style.visibility !== "hidden"
          );
        },
      );
      return visibleContainers
        .map((container) => container.shadowRoot?.textContent ?? "")
        .join("\n");
    });
  }

  walkthroughLauncher(): Locator {
    return this.page.getByTestId("walkthrough-launcher");
  }

  walkthroughDiscardButton(): Locator {
    return this.page.getByTestId("walkthrough-discard");
  }

  walkthroughDiscardConfirmation(): Locator {
    return this.page.locator('[data-testid="walkthrough-discard-confirmation"]:visible');
  }

  walkthroughFloating(): Locator {
    return this.page.getByTestId("walkthrough-floating");
  }

  walkthroughStepHeader(): Locator {
    return this.walkthroughFloating().getByTestId("walkthrough-step-header");
  }

  walkthroughStepBody(): Locator {
    return this.walkthroughFloating().getByTestId("walkthrough-step-body");
  }

  walkthroughEditorRange(): Locator {
    return this.page.getByTestId("walkthrough-editor-range");
  }

  /** PR files section within the changes panel. */
  prFilesSection(): Locator {
    return this.changes.getByTestId("pr-files-section");
  }

  /** Commits section within the changes panel (unified list of pushed + unpushed commits). */
  commitsSection(): Locator {
    return this.changes.getByTestId("commits-section");
  }

  /** Expand a collapsible section in the changes panel if currently collapsed. */
  async expandChangesSection(testId: string): Promise<void> {
    const toggle = this.changes.getByTestId(`${testId}-collapse-toggle`);
    await expect(toggle).toBeVisible({ timeout: 15_000 });
    // TimelineSection re-syncs collapsed state from defaultCollapsed until the
    // user has toggled. A late git-data update can therefore re-collapse right
    // after the first click (and can also remount the section). Retry until the
    // expanded attribute sticks instead of asserting once.
    await expect
      .poll(
        async () => {
          if ((await toggle.getAttribute("aria-expanded")) === "true") {
            return true;
          }
          await toggle.click();
          return (await toggle.getAttribute("aria-expanded")) === "true";
        },
        { timeout: 15_000 },
      )
      .toBe(true);
  }

  /** Expand the commits section (collapsed by default in the changes panel). */
  async expandCommitsSection(): Promise<void> {
    await this.expandChangesSection("commits-section");
  }

  /** Expand the PR Changes section (collapsed by default in the changes panel). */
  async expandPRChangesSection(): Promise<void> {
    await this.expandChangesSection("pr-changes-section");
  }

  /**
   * Types a message into the TipTap chat input and sends it.
   * Default submit key is Cmd+Enter (chatSubmitKey = "cmd_enter").
   * TipTap maps "Mod" to Meta on macOS and Control on Linux/Windows.
   */
  async sendMessage(text: string) {
    const editor = await this.composerReady();
    await this.waitForDirectInput();
    await editor.click();
    await editor.fill(text);
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);
  }

  /**
   * Type and submit a chat message via the Send button. Mobile (touch) layouts
   * don't submit on Ctrl/Cmd+Enter, so mobile specs use this instead.
   */
  async sendMessageViaButton(text: string) {
    const editor = await this.composerReady();
    await this.waitForDirectInput();
    await editor.click();
    await editor.fill(text);
    const isTouch = await this.page.evaluate(() => window.matchMedia("(pointer: coarse)").matches);
    if (isTouch) {
      await this.tapSubmitWhenReady();
      return;
    }
    await this.clickSubmitWhenReady();
  }

  /**
   * Wait until the composer is in direct-input mode — the idle placeholder
   * ("Continue working on the task...") visible, not the queue affordance
   * ("Queue instructions to the agent...").
   *
   * The submit button stays enabled while the session is busy (the queue
   * affordance lets you type-and-queue), and `composerReady()` only checks
   * editability, so a send right after a turn completes can race the store's
   * RUNNING→WAITING_FOR_INPUT transition and silently queue the message
   * instead of delivering it. Gating the send on the idle placeholder makes
   * sends to an idle session deterministic: typing only starts once the store
   * session state is genuinely promptable. Must run on an empty editor — the
   * placeholder decoration is only rendered while the editor has no content.
   */
  async waitForDirectInput(timeout = 15_000) {
    await this.waitForChatIdle({ timeout, requireEditable: true });
  }

  /** The composer's send/submit button (scoped to the active chat panel). */
  submitButton(): Locator {
    return this.activeChat().getByTestId("submit-message-button");
  }

  /**
   * Tap the submit button only once it is actually enabled.
   *
   * The button renders a spinner and is `disabled` while the composer is in a
   * transient not-ready state (`isSending`/`isStarting`/`isMoving`) — most
   * commonly the brief STARTING lifecycle an auto-started session passes through
   * right after it first goes idle. Acting on the button during that window is a
   * no-op tap that silently drops the message, so we gate on `toBeEnabled`
   * (waiting for the `disabled` attribute to clear — a condition, not a longer
   * fixed delay) before tapping. Mirrors `clickSubmitWhenReady` for desktop.
   */
  async tapSubmitWhenReady() {
    const submit = this.submitButton();
    await expect(submit).toBeEnabled();
    await submit.tap();
  }

  /** Desktop analog of `tapSubmitWhenReady` (uses click instead of tap). */
  async clickSubmitWhenReady() {
    const submit = this.submitButton();
    await expect(submit).toBeEnabled();
    await submit.click();
  }

  /**
   * Resolve the active chat's ProseMirror composer and wait until it is
   * ready for a normal prompt before returning it.
   *
   * TipTap uses `immediatelyRender: false`, so `EditorContent` mounts the
   * `.tiptap.ProseMirror` node only after the editor instance is created in a
   * post-mount effect; until then the contenteditable host is absent or still
   * `contenteditable="false"`. Startup now intentionally leaves that host
   * editable while the submit button remains disabled, so editability alone
   * no longer proves that a prompt can be sent. Wait for the idle placeholder
   * and editable host together, which also handles callers that only waited for
   * the chat panel to mount.
   */
  async composerReady(): Promise<Locator> {
    await this.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    const editor = this.activeChat().locator('.tiptap.ProseMirror[contenteditable="true"]').first();
    await expect(editor).toBeEditable();
    return editor;
  }

  /** Wait for the agent reply containing `text` at the given 0-based match `index`. */
  async expectChatResponseVisible(text: string, index = 0, opts: { timeout?: number } = {}) {
    const timeout = opts.timeout ?? 30_000;
    const target = () => this.activeChat().getByText(text, { exact: false }).nth(index);
    await expect(target()).toBeVisible({ timeout });
  }

  /** Toggle plan mode on/off by clicking the plan mode toggle button in the toolbar.
   *
   * Waits for the button to advertise `data-plan-available="true"` before clicking.
   * Without this gate the click can fire before `useSessionMcp` has resolved
   * `supports_mcp` for the session's agent profile (e.g. the agent type data
   * hasn't propagated into `settingsAgents.items` yet). The button is always
   * rendered, but with `planModeAvailable=false` the click only toggles the
   * plan layout — it does NOT enable plan mode on the chat input, so the
   * downstream `planModeInput()` assertion would time out for a race rather
   * than a real bug.
   */
  async togglePlanMode() {
    const btn = this.page.getByTestId("plan-mode-toggle-button");
    await expect(btn).toBeVisible({ timeout: 10_000 });
    await expect(btn).toHaveAttribute("data-plan-available", "true", { timeout: 10_000 });
    await btn.click();
  }

  /**
   * Wait until the shell terminal panel's "Connecting terminal..." overlay
   * disappears — i.e. the WebSocket actually opened for that env terminal.
   * Use this to detect the "terminal hangs forever on Connecting" bug.
   */
  async expectTerminalConnected(timeout = TERMINAL_READY_TIMEOUT): Promise<void> {
    await this.activePanel("terminal-panel")
      .getByTestId("passthrough-loading")
      .waitFor({ state: "hidden", timeout });
  }

  /** Wait for the terminal WebSocket to connect, then type a command and press Enter. */
  async typeInTerminal(command: string): Promise<void> {
    await this.expectTerminalConnected();

    const xterm = this.activePanel("terminal-panel").locator(".xterm");
    await expect(xterm).toBeVisible();
    await xterm.click();
    await this.page.keyboard.type(command);
    await this.page.keyboard.press("Enter");
  }

  /**
   * Assert the terminal buffer contains the given text.
   */
  async expectTerminalHasText(text: string): Promise<void> {
    await expect
      .poll(async () => (await this.readXtermBuffer("terminal-panel")).includes(text), {
        timeout: 10_000,
        message: `Expected terminal to contain "${text}"`,
      })
      .toBe(true);
  }

  /**
   * Click the maximize button on the dockview group that contains a tab
   * with the given name. Defaults to "Terminal".
   */
  async clickMaximize(tabName = "Terminal"): Promise<void> {
    const header = this.page.locator(
      `.dv-tabs-and-actions-container:has(.dv-default-tab:has-text('${tabName}'))`,
    );
    await header.getByTestId("dockview-maximize-btn").click();
  }

  /**
   * Assert the layout is in maximized state: terminal visible,
   * sidebar visible (UI: |sidebar|maximized-group|), chat and files hidden.
   */
  async expectMaximized(): Promise<void> {
    await expect(this.terminal).toBeVisible({ timeout: 10_000 });
    await expect(this.sidebar).toBeVisible();
    await expect(this.chat).not.toBeVisible({ timeout: 5_000 });
    await expect(this.files).not.toBeVisible({ timeout: 5_000 });
  }

  /**
   * Move the task to a workflow step through whichever stepper presentation
   * the responsive top bar selected. Narrow layouts expose the target in the
   * compact disclosure instead of rendering every step in the top bar.
   */
  async moveToWorkflowStep(step: { id: string; name: string }): Promise<void> {
    const fullStep = this.page
      .locator(`[data-testid=${JSON.stringify(`workflow-step-${step.name}`)}]:visible`)
      .first();
    const compactStepper = this.page.getByTestId("workflow-stepper-minimal");
    let presentation: "full" | "compact" | undefined;
    await expect
      .poll(
        async () => {
          if (await fullStep.isVisible()) {
            presentation = "full";
            return true;
          }
          if (await compactStepper.isVisible()) {
            presentation = "compact";
            return true;
          }
          return false;
        },
        { timeout: 10_000, message: `Waiting for workflow step presentation for ${step.name}` },
      )
      .toBe(true);

    if (presentation === "full") {
      await fullStep.hover();
      await this.page.getByRole("button", { name: "Move here", exact: true }).click();
      return;
    }

    await compactStepper.click();
    const moveButton = this.page.getByTestId(`workflow-step-disclosure-move-${step.id}`);
    await expect(moveButton).toBeVisible();
    await moveButton.click();
  }

  /**
   * Assert the layout is in the default (non-maximized) state:
   * chat, terminal, files, and sidebar are all visible, and layout fills the viewport.
   */
  async expectDefaultLayout(): Promise<void> {
    await expect(this.chat).toBeVisible({ timeout: 10_000 });
    await expect(this.terminal).toBeVisible({ timeout: 10_000 });
    await expect(this.files).toBeVisible({ timeout: 10_000 });
    await expect(this.sidebar).toBeVisible();
    await this.expectNoLayoutGap();
  }

  /**
   * Wait until the dockview api is exposed on `window` and reports at least
   * one group with a positive width. Use this as a layout-ready gate for tests
   * that assert on layout state but don't need the agent to be idle (the
   * agent may keep cycling Starting → idle → Starting under workflow
   * auto-start, never settling within a single polling window).
   */
  async waitForDockviewReady(timeout = 15_000): Promise<void> {
    await expect
      .poll(
        async () => {
          return this.page.evaluate(() => {
            type Group = { id: string; width: number };
            type Api = { groups: Group[] };
            const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
            if (!api) return false;
            return api.groups.some((g) => g.width > 1);
          });
        },
        { timeout, message: "Waiting for dockview api with positive-width groups" },
      )
      .toBe(true);
  }

  /**
   * Assert the live dockview groups all have positive widths and that the sum
   * of the root-level column widths is approximately equal to the api width.
   * Catches "central group has zero/wrong width" corruption that persists
   * across task switches when a corrupted layout is saved to per-session storage.
   */
  async expectLayoutHealthy(): Promise<void> {
    const result = await this.page.evaluate(() => {
      type Group = { id: string; width: number; height: number };
      type Api = { width: number; height: number; groups: Group[] };
      const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
      if (!api) return { error: "dockview api not exposed" };
      const bad = api.groups.filter((g) => !(g.width > 1));
      const totalWidth = api.groups.reduce((s, g) => s + (g.width > 0 ? g.width : 0), 0);
      return {
        apiWidth: api.width,
        groups: api.groups.map((g) => ({ id: g.id, width: g.width })),
        badCount: bad.length,
        totalWidth,
      };
    });
    expect(result.error, result.error).toBeUndefined();
    expect(
      result.badCount,
      `Found ${result.badCount} dockview groups with width <= 1: ${JSON.stringify(result.groups)}`,
    ).toBe(0);
    // Sum of group widths should match api width within a small rounding tolerance.
    // Note: groups can be stacked vertically so totalWidth may exceed apiWidth (one column,
    // multiple groups) — only flag if totalWidth is much smaller than apiWidth (squished).
    expect(
      result.totalWidth! >= (result.apiWidth ?? 0) - 4,
      `Total group widths (${result.totalWidth}) much smaller than api width (${result.apiWidth}): ${JSON.stringify(result.groups)}`,
    ).toBe(true);
  }

  /**
   * Assert the dockview layout columns fill the container with no large empty gap.
   * Catches bugs where columns don't expand after api.fromJSON() + setConstraints
   * (e.g. missing api.layout() call).
   */
  async expectNoLayoutGap(maxGapPx = 20): Promise<void> {
    await expect
      .poll(
        async () => {
          return this.page.evaluate((maxGap: number) => {
            const dv = document.querySelector(".dv-dockview");
            if (!dv) return false;
            const dvRect = dv.getBoundingClientRect();
            // Find the rightmost edge among all top-level column views
            const views = dv.querySelectorAll(
              ".dv-split-view-container.dv-horizontal > .dv-view-container > .dv-view",
            );
            if (views.length === 0) return false;
            let maxRight = 0;
            for (const v of views) {
              const r = v.getBoundingClientRect();
              if (r.width > 0) maxRight = Math.max(maxRight, r.right);
            }
            return dvRect.right - maxRight <= maxGap;
          }, maxGapPx);
        },
        { timeout: 5_000, message: "Layout has an empty gap on the right side (squished layout)" },
      )
      .toBe(true);
  }

  /** Git operation error message in chat (shown when a git operation fails). */
  gitOperationErrorMessage(): Locator {
    return this.chat.locator("div:has([data-testid='git-fix-button'])").first();
  }

  /** Fix button on a git operation error message. */
  gitFixButton(): Locator {
    return this.chat.getByTestId("git-fix-button");
  }

  /** Locator for the VS Code dockview tab. */
  vscodeTab(): Locator {
    return this.page.locator(".dv-default-tab:has-text('VS Code')");
  }

  /** Locator for the VS Code code-server iframe. */
  vscodeIframe(): Locator {
    return this.page.locator('iframe[title="VS Code"]');
  }

  // --- New Session Dialog ---

  /** "+" button in the dockview header to open the add-panel dropdown. */
  addPanelButton(): Locator {
    return this.page.getByTestId("dockview-add-panel-btn").first();
  }

  /** Open a blank built-in Browser panel from the dockview + menu. */
  async addBrowserPanel(): Promise<void> {
    await this.addPanelButton().click();
    await this.page.getByRole("menuitem", { name: "Browser", exact: true }).click();
  }

  /** "New Session" menu item in the dockview + dropdown. */
  newSessionMenuButton(): Locator {
    return this.page.getByTestId("new-session-button");
  }

  /** Row in the dockview "+" add-panel menu for a registered plugin task panel. */
  addPanelPluginItem(pluginId: string, panelId: string): Locator {
    return this.page.getByTestId(`add-panel-plugin-item-${pluginId}-${panelId}`);
  }

  /** Open the new session dialog via the + menu. */
  async openNewSessionDialog(): Promise<void> {
    await this.addPanelButton().click();
    await this.newSessionMenuButton().click();
  }

  /** New session or handoff dialog container. */
  sessionLaunchDialog(): Locator {
    return this.page.getByRole("dialog").filter({ hasText: /New agent in|Hand off to/ });
  }

  /** The new session dialog container. */
  newSessionDialog(): Locator {
    return this.page.getByRole("dialog").filter({ hasText: "New agent in" });
  }

  /** Handoff dialog opened from session tab context menu. */
  handoffDialog(): Locator {
    return this.page.getByRole("dialog").filter({ hasText: "Hand off to" });
  }

  /** Prompt textarea inside the new session or handoff dialog. */
  newSessionPromptInput(): Locator {
    return this.sessionLaunchDialog().getByTestId("task-description-input");
  }

  /** Start Agent button inside the new session or handoff dialog. */
  newSessionStartButton(): Locator {
    return this.sessionLaunchDialog().getByRole("button", { name: "Start Agent" });
  }

  /** Environment info badges inside the new session dialog. */
  newSessionEnvironmentInfo(): Locator {
    return this.sessionLaunchDialog().getByText("Same environment as current session");
  }

  /** Handoff submenu trigger in session context or actions menu. */
  handoffSubmenu(): Locator {
    return this.page.getByTestId("session-handoff-submenu");
  }

  /** Handoff profile item in the Handoff submenu. */
  handoffProfileItem(profileId: string): Locator {
    return this.page.getByTestId(`handoff-profile-${profileId}`);
  }

  /** Open handoff dialog via session tab right-click context menu. */
  async openHandoffDialog(sessionId: string, profileId: string): Promise<void> {
    await this.sessionTabBySessionId(sessionId).click({ button: "right" });
    await this.handoffSubmenu().hover();
    await this.handoffProfileItem(profileId).click();
  }

  /** Open handoff dialog via mobile session row actions menu. */
  async openMobileHandoffDialog(sessionId: string, profileId: string): Promise<void> {
    await this.page.getByTestId("mobile-sessions-pill").click();
    const row = this.page.getByTestId(`mobile-session-row-${sessionId}`);
    await row.getByRole("button", { name: "Session actions" }).click();
    await this.handoffSubmenu().hover();
    await this.handoffProfileItem(profileId).click();
  }

  /** Open the New Agent dialog from the phone session controls. */
  async openMobileNewSessionDialog(): Promise<void> {
    await this.page.getByTestId("mobile-sessions-pill").tap();
    await this.page.getByTestId("mobile-launch-session").tap();
  }

  /** Session tab in dockview by session label (e.g., "Session 1", "Session 2"). */
  sessionTab(label: string): Locator {
    return this.page.locator(`.dv-default-tab:has-text('${label}')`);
  }

  /** Session item in the + dropdown's reopen list by session ID. */
  sessionReopenItem(sessionId: string): Locator {
    return this.page.getByTestId(`reopen-session-${sessionId}`);
  }

  /** All session reopen items in the + dropdown. */
  sessionReopenItems(): Locator {
    return this.page.locator("[role='menuitem'][data-testid^='reopen-session-']");
  }

  /** All session tabs in dockview (panels using the sessionTab tab component). */
  sessionTabs(): Locator {
    return this.page.locator(".dv-default-tab").filter({
      has: this.page.locator("[data-testid^='reopen-session-'], .tabler-icon-star").first(),
    });
  }

  /** Dockview session tab matched by partial text (e.g., "Mock Agent" or index "1"). */
  sessionTabByText(text: string): Locator {
    return this.page.locator(`[data-testid^='session-tab-']:has-text('${text}')`);
  }

  /** Session tab container identified by session ID (data-testid="session-tab-{id}"). */
  sessionTabBySessionId(sessionId: string): Locator {
    return this.page.getByTestId(`session-tab-${sessionId}`);
  }

  /** Dockview close (X) button inside a session tab. */
  sessionTabCloseButton(sessionId: string): Locator {
    return this.page.getByTestId(`session-tab-close-${sessionId}`);
  }

  /** Context menu on a dockview tab — right-click the tab to trigger it. */
  async rightClickTab(text: string): Promise<void> {
    const tab = this.page.locator(`[data-testid^='session-tab-']:has-text('${text}')`);
    await tab.click({ button: "right" });
  }

  /** Right-click the first session tab (useful when there is only one session). */
  async rightClickFirstSessionTab(): Promise<void> {
    const tab = this.page.locator("[data-testid^='session-tab-']").first();
    await tab.click({ button: "right" });
  }

  /** Context menu item by visible label. */
  contextMenuItem(label: string): Locator {
    return this.page.getByRole("menuitem", { name: label });
  }

  /** Alert dialog (e.g., delete confirmation). */
  alertDialog(): Locator {
    return this.page.getByRole("alertdialog");
  }

  /** Primary star icon inside a dockview session tab. The star is rendered as a
   *  sibling of `.dv-default-tab` inside the `data-testid="session-tab-<id>"`
   *  wrapper, so we anchor on that wrapper rather than `.dv-default-tab` itself. */
  primaryStarInTab(text: string): Locator {
    return this.sessionTabByText(text).locator(".tabler-icon-star").first();
  }

  /** Primary star icon inside a session tab identified by its session ID. */
  primaryStarInSessionTab(sessionId: string): Locator {
    return this.sessionTabBySessionId(sessionId).locator(".tabler-icon-star").first();
  }

  /** "Move to next step" button in the chat status bar. */
  proceedNextStepButton(): Locator {
    return this.page.getByTestId("proceed-next-step");
  }

  /** Click a task in the sidebar by title. */
  async clickTaskInSidebar(title: string): Promise<void> {
    const taskRow = this.sidebar.locator("[role='button']").filter({ hasText: title });
    await taskRow.click();
  }

  // --- File tree multi-select helpers ---

  /** Find a tree node by its data-path attribute. */
  fileTreeNode(nodePath: string): Locator {
    return this.fileTree.fileTreeNode(nodePath);
  }

  /** Visible search button in the Files panel. */
  fileSearchButton(): Locator {
    return this.fileTree.fileSearchButton();
  }

  /** Search input shown in the visible Files panel. */
  fileSearchInput(): Locator {
    return this.fileTree.fileSearchInput();
  }

  /** Search result by its task-root-relative path. */
  fileSearchResult(nodePath: string): Locator {
    return this.fileTree.fileSearchResult(nodePath);
  }

  /** All file tree nodes with data-selected="true". */
  fileTreeSelectedNodes(): Locator {
    return this.fileTree.fileTreeSelectedNodes();
  }

  /** The desktop context-menu action for the selected file-tree node. */
  fileTreeAddToChatContextMenuItem(): Locator {
    return this.fileTree.fileTreeAddToChatContextMenuItem();
  }

  /** Visible coarse-pointer row action for one file-tree node. */
  fileTreeNodeActions(nodePath: string): Locator {
    return this.fileTree.fileTreeNodeActions(nodePath);
  }

  /** Responsive dropdown opened from a file-tree row action. */
  fileTreeTouchMenu(): Locator {
    return this.fileTree.fileTreeTouchMenu();
  }

  /** Add-to-chat item inside the responsive file-tree dropdown. */
  fileTreeTouchAddToChatContextItem(): Locator {
    return this.fileTree.fileTreeTouchAddToChatContextItem();
  }

  /** Pending composer chip for a file or directory path. */
  chatContextFile(path: string): Locator {
    return this.fileTree.chatContextFile(path);
  }

  /** Context-file badge on a sent user message. */
  sentMessageContextFile(path: string): Locator {
    return this.fileTree.sentMessageContextFile(path);
  }

  // --- Changes panel multi-select helpers ---

  /** Find a file row in the changes panel by path. */
  changesFileRow(path: string): Locator {
    return this.changes.locator(`[data-changes-file="${path}"]`);
  }

  /** All selected file rows in the changes panel. */
  changesSelectedRows(): Locator {
    return this.changes.locator("[data-selected='true']");
  }

  /** All file rows in the changes panel currently marked as the active tab. */
  changesActiveRows(): Locator {
    return this.changes.locator("[data-active='true']");
  }

  /**
   * Close every file-diff panel in dockview: the `preview:file-diff` slot AND
   * any pinned `diff:file:<path>` panels created by promoting the preview.
   * After this resolves, no diff tab is active so the changes-panel rows
   * settle to `data-active="false"`.
   */
  async closeFileDiffPreview(): Promise<void> {
    await this.page.evaluate(() => {
      type PanelApi = { close: () => void };
      type Panel = { id: string; api: PanelApi };
      type Api = { panels: Panel[]; getPanel: (i: string) => Panel | undefined };
      const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
      if (!api) return;
      api.getPanel("preview:file-diff")?.api.close();
      // Snapshot before iterating: panel.api.close() mutates api.panels in
      // place, so iterating the live array would skip every other panel.
      const pinned = [...api.panels].filter((p) => p.id.startsWith("diff:file:"));
      for (const panel of pinned) panel.api.close();
    });
  }

  /** Bulk action bar for a variant (unstaged/staged). */
  changesBulkActionBar(variant: "unstaged" | "staged"): Locator {
    return this.changes.getByTestId(`bulk-actions-${variant}`);
  }

  /** Bulk stage button (unstaged section). */
  changesBulkStageButton(): Locator {
    return this.changes.getByTestId("bulk-stage");
  }

  /** Bulk unstage button (staged section). */
  changesBulkUnstageButton(): Locator {
    return this.changes.getByTestId("bulk-unstage-staged");
  }

  /** Bulk discard button for a variant. */
  changesBulkDiscardButton(variant: "unstaged" | "staged" = "unstaged"): Locator {
    return this.changes.getByTestId(`bulk-discard-${variant}`);
  }

  // --- Plan revisions / rewind ---

  /** Rewind button in the plan panel header (opens revision history popover). */
  rewindButton(): Locator {
    return this.planPanel.getByTestId("plan-rewind-button");
  }

  /** Plan revisions popover (opens after clicking rewind). */
  revisionsPopover(): Locator {
    return this.page.getByTestId("plan-revisions-popover");
  }

  /** All revision rows inside the popover, newest-first. */
  revisionRows(): Locator {
    return this.revisionsPopover().getByTestId("plan-revision-row");
  }

  /** Specific revision row by number. */
  revisionRow(n: number): Locator {
    return this.revisionsPopover().locator(`[data-revision-number="${n}"]`);
  }

  /** Revert button scoped to a given revision row. */
  revertButton(row: Locator): Locator {
    return row.getByTestId("plan-revision-revert-button");
  }

  /** Revert-confirm dialog. */
  revertConfirmDialog(): Locator {
    return this.page.getByTestId("plan-revert-confirm-dialog");
  }

  /** Desktop row-local restore confirmation popover. */
  revertConfirmPopover(): Locator {
    return this.page.getByTestId("plan-revision-restore-confirm-popover");
  }

  revertConfirmPopoverOk(): Locator {
    return this.revertConfirmPopover().getByTestId("plan-revision-restore-confirm");
  }

  revertConfirmPopoverCancel(): Locator {
    return this.revertConfirmPopover().getByRole("button", { name: "Cancel", exact: true });
  }

  /** Phone row-local restore confirmation. */
  revertInlineConfirmation(row: Locator): Locator {
    return row.getByTestId("plan-revision-restore-inline-confirmation");
  }

  revertInlineConfirm(row: Locator): Locator {
    return row.getByTestId("plan-revision-restore-confirm");
  }

  revertInlineCancel(row: Locator): Locator {
    return this.revertInlineConfirmation(row).getByRole("button", {
      name: "Cancel",
      exact: true,
    });
  }

  revertConfirmOk(): Locator {
    return this.page.getByTestId("plan-revert-confirm-ok");
  }

  revertConfirmCancel(): Locator {
    return this.page.getByTestId("plan-revert-confirm-cancel");
  }

  /** TipTap editor inside the plan panel (for typing user edits). */
  planEditor(): Locator {
    return this.planPanel.locator(".ProseMirror");
  }

  /** Open the rewind popover and wait for it to render. No-op when already open. */
  async openRewind(): Promise<void> {
    // Radix keeps the closed content in the DOM during its exit transition,
    // and mobile can report that content as visible while the trigger is
    // already closed. The trigger state is the authoritative open signal.
    if ((await this.rewindButton().getAttribute("aria-expanded")) === "true") return;
    await this.rewindButton().click();
    await expect(this.revisionsPopover()).toBeVisible({ timeout: 5_000 });
  }

  /** Open rewind, click revert on the row with the given revision number, and confirm. */
  async revertToRevision(n: number): Promise<void> {
    await this.openRewind();
    await this.revertButton(this.revisionRow(n)).click();
    await expect(this.revertConfirmPopover()).toBeVisible({ timeout: 5_000 });
    await this.revertConfirmPopoverOk().click();
  }

  // --- Plan revision preview & compare (Phase 6) ---

  /** Click the row body (not the Revert/Compare buttons) to open the preview dialog. */
  revisionRowBody(row: Locator): Locator {
    return row.getByTestId("plan-revision-row-body");
  }

  previewDialog(): Locator {
    return this.page.getByTestId("plan-revision-preview-dialog");
  }

  previewBody(): Locator {
    return this.page.getByTestId("plan-revision-preview-body");
  }

  previewRestoreButton(): Locator {
    return this.page.getByTestId("plan-revision-preview-restore");
  }

  previewCompareWithCurrentButton(): Locator {
    return this.page.getByTestId("plan-revision-preview-compare-with-current");
  }

  previewCompareWithPreviousButton(): Locator {
    return this.page.getByTestId("plan-revision-preview-compare-with-previous");
  }

  previewCloseButton(): Locator {
    return this.page.getByTestId("plan-revision-preview-close");
  }

  diffDialog(): Locator {
    return this.page.getByTestId("plan-revision-diff-dialog");
  }

  diffSummary(): Locator {
    return this.page.getByTestId("plan-revision-diff-summary");
  }

  diffLines(kind?: "add" | "remove" | "context"): Locator {
    const root = this.diffDialog();
    if (!kind) return root.getByTestId("plan-revision-diff-line");
    return root.locator(`[data-testid="plan-revision-diff-line"][data-line-kind="${kind}"]`);
  }

  diffSplitCells(kind?: "add" | "remove" | "context" | "empty"): Locator {
    const root = this.diffDialog();
    if (!kind) return root.getByTestId("plan-revision-diff-split-cell");
    return root.locator(`[data-testid="plan-revision-diff-split-cell"][data-line-kind="${kind}"]`);
  }

  diffModeToggle(mode: "unified" | "split"): Locator {
    return this.page.getByTestId(`plan-revision-diff-mode-${mode}`);
  }

  diffRestoreButton(): Locator {
    return this.page.getByTestId("plan-revision-diff-restore");
  }

  diffCloseButton(): Locator {
    return this.page.getByTestId("plan-revision-diff-close");
  }

  /** Open rewind and click into the row body to bring up the preview dialog. */
  async openRevisionPreview(n: number): Promise<void> {
    await this.openRewind();
    await this.revisionRowBody(this.revisionRow(n)).click();
    await expect(this.previewDialog()).toBeVisible({ timeout: 5_000 });
  }

  // --- Panel search helpers (Ctrl+F feature) ---

  /** Any currently-mounted panel search bar. */
  panelSearchBar(): Locator {
    return this.page.locator("[data-panel-search-bar]");
  }

  /** Search input inside the currently-mounted bar. */
  panelSearchInput(): Locator {
    return this.panelSearchBar().locator('input[type="text"]');
  }

  /** "N / M" match counter. */
  panelSearchCounter(): Locator {
    return this.panelSearchBar().locator('[aria-live="polite"]');
  }
}
