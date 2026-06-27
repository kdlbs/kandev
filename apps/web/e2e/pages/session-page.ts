import { type Locator, type Page, expect } from "@playwright/test";

/** Maps old state-section labels to the new per-task state icon data-testid. */
function sectionLabelToStateTestId(label: string): string {
  if (label === "Running") return "task-state-running";
  if (label === "Turn Finished") return "task-state-review";
  return "task-state-backlog";
}

export class SessionPage {
  readonly chat: Locator;
  readonly sidebar: Locator;
  readonly terminal: Locator;
  readonly files: Locator;
  readonly changes: Locator;
  readonly planPanel: Locator;
  readonly stepper: Locator;
  readonly passthroughTerminal: Locator;

  constructor(private readonly page: Page) {
    this.chat = page.getByTestId("session-chat");
    this.sidebar = page.getByTestId("task-sidebar");
    this.terminal = page.getByTestId("terminal-panel");
    this.files = page.getByTestId("files-panel");
    this.changes = page.getByTestId("changes-panel");
    this.planPanel = page.getByTestId("plan-panel");
    this.stepper = page.getByTestId("workflow-stepper");
    this.passthroughTerminal = page.getByTestId("passthrough-terminal");
  }

  // Port forward dialog locators
  get portForwardButton() {
    return this.page.getByTestId("port-forward-button");
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

  // Chat status bar locators
  chatStatusBar() {
    return this.page.getByTestId("chat-status-bar");
  }
  prMergedBanner() {
    return this.page.getByTestId("pr-merged-banner");
  }
  prMergedArchiveButton() {
    return this.page.getByTestId("pr-merged-archive-button");
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

  private chatEditor(): Locator {
    return this.activeChat().locator(".tiptap.ProseMirror").first();
  }

  private sessionTabTriggers(): Locator {
    return this.page.locator(
      '[data-testid^="session-tab-"]:not([data-testid^="session-tab-close-"])',
    );
  }

  private visibleSessionTabTriggers(): Locator {
    return this.page.locator(
      '[data-testid^="session-tab-"]:not([data-testid^="session-tab-close-"]):visible',
    );
  }

  private visibleSessionDockTabs(): Locator {
    return this.page.locator(".dv-default-tab").filter({
      has: this.visibleSessionTabTriggers(),
    });
  }

  private visibleChatDockTabs(): Locator {
    return this.page.locator(".dv-default-tab:visible").filter({ hasText: /^Chat$/ });
  }

  private currentRouteSessionId(): string | null {
    try {
      return new URL(this.page.url()).searchParams.get("sessionId");
    } catch {
      return null;
    }
  }

  private currentRouteSessionTabTrigger(): Locator | null {
    const sessionId = this.currentRouteSessionId();
    return sessionId ? this.page.getByTestId(`session-tab-${sessionId}`) : null;
  }

  private currentRouteSessionDockTab(): Locator | null {
    const routeTab = this.currentRouteSessionTabTrigger();
    return routeTab ? this.page.locator(".dv-default-tab").filter({ has: routeTab }).first() : null;
  }

  private sessionTabClickCandidates(): Locator[] {
    const candidates: Locator[] = [];
    const routeDockTab = this.currentRouteSessionDockTab();
    const routeTrigger = this.currentRouteSessionTabTrigger();
    if (routeDockTab) candidates.push(routeDockTab);
    if (routeTrigger) candidates.push(routeTrigger);
    candidates.push(this.visibleChatDockTabs().first());
    candidates.push(this.visibleSessionDockTabs().first());
    candidates.push(this.visibleSessionTabTriggers().first());
    return candidates;
  }

  private async clickFirstAvailableSessionTab(timeout = 15_000): Promise<void> {
    let lastError: unknown;
    const candidateTimeout = Math.min(timeout, 5_000);

    for (const tab of this.sessionTabClickCandidates()) {
      try {
        await tab.waitFor({ state: "visible", timeout: candidateTimeout });
        await tab.click();
        return;
      } catch (err) {
        lastError = err;
      }
    }

    if (lastError instanceof Error) throw lastError;
    throw new Error("no session tab could be clicked");
  }

  private async foregroundSessionChatFromTab(timeout = 15_000): Promise<void> {
    let lastError: unknown;
    const candidateTimeout = Math.min(timeout, 5_000);

    for (const tab of this.sessionTabClickCandidates()) {
      try {
        await tab.waitFor({ state: "visible", timeout: candidateTimeout });
        await tab.click();
        await this.activeChat().waitFor({ state: "visible", timeout: candidateTimeout });
        return;
      } catch (err) {
        lastError = err;
      }
    }

    try {
      await this.foregroundSessionChatWithDockviewApi(timeout);
      return;
    } catch (err) {
      lastError = err;
    }

    if (lastError instanceof Error) throw lastError;
    throw new Error("session chat did not become visible");
  }

  private async foregroundSessionChatWithDockviewApi(timeout = 15_000): Promise<void> {
    const getPanelIds = async () =>
      this.page.evaluate(() => {
        type PanelApi = { setActive: () => void };
        type Panel = { id: string; api: PanelApi };
        type Api = {
          activePanel?: Panel;
          panels: Panel[];
          getPanel: (id: string) => Panel | undefined;
        };
        const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
        if (!api) return [];

        const seen = new Set<string>();
        const ids: string[] = [];
        const add = (id: string | null | undefined) => {
          if (!id || seen.has(id) || !api.getPanel(id)) return;
          seen.add(id);
          ids.push(id);
        };

        const sessionId = new URL(window.location.href).searchParams.get("sessionId");
        add(sessionId ? `session:${sessionId}` : undefined);
        if (api.activePanel?.id.startsWith("session:")) add(api.activePanel.id);
        for (const panel of api.panels) {
          if (panel.id.startsWith("session:")) add(panel.id);
        }
        add("chat");

        return ids;
      });

    await expect
      .poll(async () => (await getPanelIds()).length, {
        timeout,
        message: "Finding session chat panel through dockview api",
      })
      .toBeGreaterThan(0);

    let lastError: unknown;
    for (const panelId of await getPanelIds()) {
      try {
        const activated = await this.page.evaluate((id) => {
          type PanelApi = { setActive: () => void };
          type Panel = { id: string; api: PanelApi };
          type Api = { getPanel: (id: string) => Panel | undefined };
          const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
          const panel = api?.getPanel(id);
          if (!panel) return false;
          panel.api.setActive();
          return true;
        }, panelId);
        if (!activated) continue;
        await this.activeChat().waitFor({ state: "visible", timeout: Math.min(timeout, 5_000) });
        return;
      } catch (err) {
        lastError = err;
      }
    }

    if (lastError instanceof Error) throw lastError;
    throw new Error("session chat did not become visible through dockview api");
  }

  private async waitForEditableChatEditor(timeout = 15_000): Promise<Locator> {
    const editor = this.chatEditor();
    await expect(editor).toBeVisible({ timeout });
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout });
    return editor;
  }

  async waitForLoad(timeout = 15_000) {
    // When multiple session tabs are open, multiple session-chat panels exist in
    // the DOM but only the active one is visible. Use :visible to avoid matching
    // a hidden background panel (which would cause the wait to time out).
    await this.activeChat()
      .waitFor({ state: "visible", timeout })
      .catch(() => this.showSessionContext(timeout));
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
    await this.waitForDockviewReady(Math.min(timeout, 15_000)).catch(() => undefined);
    if (await this.activeChat().isVisible()) return;

    let lastError: unknown;
    for (const reloadFirst of [false, true]) {
      try {
        if (reloadFirst) await this.page.reload();
        // Clicking a tab that's already active is harmless; clicking a background
        // one promotes its panel to the foreground.
        await this.foregroundSessionChatFromTab(timeout);
        return;
      } catch (err) {
        lastError = err;
      }
    }

    if (lastError instanceof Error) throw lastError;
    throw new Error("session chat did not become visible");
  }

  /**
   * Wait for the chat to be idle (input placeholder visible, agent not busy).
   *
   * On mobile-chrome (and occasionally desktop), there's a WS subscribe race:
   * a fresh task auto-starts its agent, the mock agent completes in <1s, and
   * the session_state transition (RUNNING -> AWAITING_INPUT) can fan out
   * before the client's WS subscription registers server-side. The client
   * then sits with `isAgentBusy=true` forever and the idle placeholder
   * never renders. SSR picks up the right state on the next page load, so
   * one targeted reload-and-retry is enough to recover.
   *
   * After a backend restart, auto-resume can briefly surface the recovery
   * prompt ("Environment setup failed"); click through it when visible.
   *
   * This is the same race the office agent-run-live spec rides out with
   * `expect.poll`-based re-seeding.
   */
  async waitForChatIdle(opts: { timeout?: number; attemptTimeout?: number } = {}) {
    const softTotalTimeout = opts.timeout ?? 45_000;
    const attemptTimeout =
      opts.attemptTimeout ?? Math.min(15_000, Math.max(5_000, Math.floor(softTotalTimeout / 3)));
    const pollSlice = 1_500;
    const idle = this.anyIdleInput();
    const start = Date.now();
    let reloaded = false;
    const idleAndSettled = async () => {
      if (!(await idle.isVisible())) return false;
      return !(await this.activeChatAgentStatus()
        .first()
        .isVisible()
        .catch(() => false));
    };

    while (Date.now() - start < softTotalTimeout) {
      if (await idleAndSettled()) return;

      const resumeButton = this.recoveryResumeButton();
      if (await resumeButton.isVisible()) {
        if (await resumeButton.isEnabled()) {
          await resumeButton.click();
        }
        await resumeButton.waitFor({ state: "hidden", timeout: pollSlice }).catch(() => undefined);
        continue;
      }

      const elapsed = Date.now() - start;
      if (!reloaded && elapsed >= attemptTimeout) {
        reloaded = true;
        await this.page.reload();
        await this.activeChat()
          .waitFor({ state: "visible", timeout: attemptTimeout })
          .catch(() => undefined);
        continue;
      }

      const remaining = softTotalTimeout - elapsed;
      await idle
        .waitFor({ state: "visible", timeout: Math.min(pollSlice, remaining) })
        .catch(() => undefined);
    }

    await expect.poll(idleAndSettled, { timeout: 1_000 }).toBe(true);
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
   * Read the text content of an xterm.js terminal buffer.
   * xterm renders to canvas/WebGL so text isn't in the DOM. Uses the
   * __xtermReadBuffer() helper exposed on the terminal container element.
   */
  private readXtermBuffer(testId: string): Promise<string> {
    return this.page.evaluate((tid) => {
      const panel = document.querySelector(`[data-testid="${tid}"]`);
      if (!panel) return "";
      const xtermEl = panel.querySelector(".xterm");
      type XC = HTMLElement & { __xtermReadBuffer?: () => string };
      const container = xtermEl?.parentElement as XC | null | undefined;
      return container?.__xtermReadBuffer?.() ?? "";
    }, testId);
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
      await this.page.waitForTimeout(200);
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
    const testId = sectionLabelToStateTestId(label);
    return this.sidebar.getByTestId(testId).first();
  }

  /**
   * Task item in the sidebar matching both a title and a state label.
   * Accepts "Turn Finished" (review/completed), "Running" (in-progress), or "Backlog".
   */
  taskInSection(title: string, sectionLabel: string): Locator {
    const testId = sectionLabelToStateTestId(sectionLabel);
    return this.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ has: this.page.getByText(title, { exact: false }) })
      .filter({ has: this.page.getByTestId(testId) });
  }

  /** Agent STARTING or RUNNING status indicator. */
  agentStatus(): Locator {
    return this.page.getByRole("status", {
      name: /^(Agent is starting|Agent is running|Starting .+)$/,
    });
  }

  /** Divider that appears after the "New session started" status message is rendered. */
  turnComplete(): Locator {
    return this.page.getByTestId("agent-turn-complete");
  }

  /** Chat input placeholder when agent is idle (default mode). */
  idleInput(): Locator {
    return this.page.locator('[data-placeholder="Continue working on the task..."]');
  }

  /** Chat input placeholder when agent is idle in any current mode. */
  anyIdleInput(): Locator {
    return this.page
      .locator('[data-placeholder="Continue working on the task..."]')
      .or(this.page.locator('[data-placeholder="Continue working on the plan..."]'))
      .or(this.page.locator('[data-placeholder="Continue working on the file..."]'));
  }

  /** Visible agent busy status inside the active chat panel. */
  activeChatAgentStatus(): Locator {
    return this.activeChat().getByRole("status", {
      name: /^(Agent is starting|Agent is running|Starting .+)$/,
    });
  }

  /** Chat input placeholder when agent is idle (plan mode). */
  planModeInput(): Locator {
    return this.page.locator('[data-placeholder="Continue working on the plan..."]');
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

  /** A specific clarification option button by its text label. */
  clarificationOption(text: string): Locator {
    return this.clarificationOverlay()
      .getByTestId("clarification-option")
      .filter({ hasText: text });
  }

  /** Skip (X) button on the clarification overlay. */
  clarificationSkip(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-skip");
  }

  /** Custom text input on the clarification overlay. */
  clarificationInput(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-input");
  }

  /** Deferred notice shown when agent has disconnected from clarification. */
  clarificationDeferredNotice(): Locator {
    return this.clarificationOverlay().getByTestId("clarification-deferred-notice");
  }

  /** Expired notice rendered in chat history when the agent timed out waiting. */
  clarificationExpiredNotice(): Locator {
    return this.activeChat().getByTestId("clarification-expired-notice");
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
    return this.activeChat().getByTestId("permission-action-row");
  }

  /** All "Approve" buttons for pending permission requests. */
  permissionApproveButtons(): Locator {
    return this.activeChat().getByTestId("permission-approve");
  }

  /** Kandev-MCP-only "Approve" buttons (excludes the generic ToolCallMessage
   *  fallback row that may briefly duplicate the same pending_id). */
  kandevPermissionApproveButtons(): Locator {
    return this.activeChat()
      .getByTestId("kandev-tool-permission")
      .getByTestId("permission-approve");
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

  /** "Start fresh session" button shown after agent crash. */
  recoveryFreshButton(): Locator {
    return this.page.getByTestId("recovery-fresh-button");
  }

  /** "Cancel" button shown on the yellow transient-retry (529 Overloaded) card. */
  recoveryCancelRetryButton(): Locator {
    return this.page.getByTestId("recovery-cancel-retry-button");
  }

  /** The yellow "Provider overloaded — retrying…" status card text. */
  transientRetryCard(): Locator {
    return this.chat.getByText(/Provider overloaded — retrying/i);
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
   * and confirms the archive dialog.
   */
  async archiveTaskInSidebar(title: string): Promise<void> {
    await this.openSidebarMenuAndClick(title, "Archive");
    // Confirm the archive dialog
    const confirmButton = this.page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Archive" });
    await confirmButton.click();
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
        await this.page.waitForTimeout(500);
      }
    }
    // Final attempt without catch
    await taskRow.hover();
    await taskRow.getByRole("button", { name: "Task actions" }).click();
    await this.page.getByRole("menuitem", { name: itemName }).click();
  }

  stepperStep(name: string): Locator {
    return this.page.getByTestId(`workflow-step-${name}`);
  }

  /** PR button in the topbar (visible only when a PR is associated). */
  prTopbarButton(): Locator {
    return this.page.getByTestId("pr-topbar-button");
  }

  /** PR detail panel (auto-shown when task has an associated PR). */
  prDetailPanel(): Locator {
    return this.page.getByTestId("pr-detail-panel");
  }

  /** "Approve PR" button inside the PR detail panel header. Hidden when the
   * current GitHub user authored the PR (self-approval is rejected upstream). */
  prApproveButton(): Locator {
    return this.page.getByTestId("pr-approve-button");
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

  /** Multi-PR aggregate popover content (segmented tabs + selected PR's CI). */
  prTopbarPopoverAggregate(): Locator {
    return this.page.locator("[data-testid='pr-multi-popover']:visible").first();
  }

  /** A single PR tab inside the multi-PR aggregate popover, by repo + PR number. */
  prMultiPopoverTab(repo: string, prNumber: number): Locator {
    return this.page.getByTestId(`pr-popover-tab-${repo}-${prNumber}`);
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
  private async hoverPRChipTrigger(): Promise<void> {
    const chip = this.prStatusChip();
    await chip.scrollIntoViewIfNeeded();
    const box = await chip.boundingBox();
    expect(box).not.toBeNull();
    await chip.focus();
    await this.page.mouse.move(0, 0);
    await this.page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
    await chip.evaluate((element) => {
      const targets = [element.parentElement, element].filter(Boolean) as Element[];
      const mouseInit: MouseEventInit = { bubbles: true, cancelable: true, view: window };
      const pointerInit: PointerEventInit = {
        ...mouseInit,
        pointerType: "mouse",
        isPrimary: true,
      };

      for (const target of targets) {
        if ("PointerEvent" in window) {
          target.dispatchEvent(new PointerEvent("pointerover", pointerInit));
          target.dispatchEvent(
            new PointerEvent("pointerenter", { ...pointerInit, bubbles: false }),
          );
          target.dispatchEvent(new PointerEvent("pointermove", pointerInit));
        }
        target.dispatchEvent(new MouseEvent("mouseover", mouseInit));
        target.dispatchEvent(new MouseEvent("mouseenter", { ...mouseInit, bubbles: false }));
        target.dispatchEvent(new MouseEvent("mousemove", mouseInit));
      }
    });
  }

  private async hoverPRChipUntil(popover: Locator): Promise<void> {
    await expect(async () => {
      await this.hoverPRChipTrigger();
      try {
        await expect(popover).toBeVisible({ timeout: 1_500 });
        return;
      } catch {
        await this.prStatusChip().click({ force: true });
        await expect(popover).toBeVisible({ timeout: 2_000 });
      }
    }).toPass({ timeout: 10_000 });
  }

  async hoverPRChip(): Promise<void> {
    await this.hoverPRChipUntil(this.prChipPopover());
  }

  async hoverPRMultiChip(): Promise<void> {
    await this.hoverPRChipUntil(this.prTopbarPopoverAggregate());
  }

  /**
   * Assert the `pr-detail` panel's dockview group contains at least one
   * `session:{sessionId}` panel — i.e. the PR opened as a tab next to a
   * session chat, not as a split in a separate group. Regression guard for
   * the "PR opens in a split instead of the center tab" bug.
   *
   * Checks group membership of the PR panel directly rather than picking a
   * session panel first, so the assertion is deterministic even when outgoing
   * and incoming session panels briefly coexist during a task switch.
   */
  async expectPrPanelAndSessionShareGroup(): Promise<void> {
    const result = await this.page.evaluate(() => {
      type Panel = { id: string; group?: { id?: string } };
      type Api = { panels: Panel[]; getPanel: (i: string) => Panel | undefined };
      const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
      if (!api) return { error: "dockview api not exposed" };
      const pr = api.getPanel("pr-detail");
      if (!pr) return { error: "pr-detail panel missing" };
      const prGroupId = pr.group?.id ?? null;
      const sessionPanels = api.panels.filter((p) => p.id.startsWith("session:"));
      if (sessionPanels.length === 0) return { error: "no session panel" };
      const sessionInPrGroup = sessionPanels.some((p) => p.group?.id === prGroupId);
      return {
        sessionInPrGroup,
        prGroupId,
        sessionLocations: sessionPanels.map((p) => `${p.id}@${p.group?.id ?? "?"}`),
      };
    });
    expect(result.error, result.error).toBeUndefined();
    expect(
      result.sessionInPrGroup,
      `PR panel landed in a dockview group that contains no session chat. ` +
        `PR group=${result.prGroupId} sessions=[${result.sessionLocations?.join(", ")}]`,
    ).toBe(true);
  }

  /**
   * Like `expectPrPanelAndSessionShareGroup`, but matches any pr-detail panel
   * (legacy `pr-detail` or keyed `pr-detail|owner/repo/N`). Use for flows that
   * exercise the manual click path, which always creates a keyed panel.
   */
  async expectAnyPrPanelAndSessionShareGroup(): Promise<void> {
    const result = await this.page.evaluate(() => {
      type Panel = { id: string; group?: { id?: string } };
      type Api = { panels: Panel[]; getPanel: (i: string) => Panel | undefined };
      const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
      if (!api) return { error: "dockview api not exposed" };
      const prPanels = api.panels.filter(
        (p) => p.id === "pr-detail" || p.id.startsWith("pr-detail|"),
      );
      if (prPanels.length === 0) return { error: "no pr-detail panel" };
      const sessionPanels = api.panels.filter((p) => p.id.startsWith("session:"));
      if (sessionPanels.length === 0) return { error: "no session panel" };
      const sessionGroupIds = new Set(sessionPanels.map((p) => p.group?.id).filter(Boolean));
      const allPrsInSessionGroup = prPanels.every((p) => {
        const gid = p.group?.id;
        return gid !== undefined && sessionGroupIds.has(gid);
      });
      return {
        allPrsInSessionGroup,
        prLocations: prPanels.map((p) => `${p.id}@${p.group?.id ?? "?"}`),
        sessionLocations: sessionPanels.map((p) => `${p.id}@${p.group?.id ?? "?"}`),
      };
    });
    expect(result.error, result.error).toBeUndefined();
    expect(
      result.allPrsInSessionGroup,
      `PR panel(s) landed outside the session's group. ` +
        `prs=[${result.prLocations?.join(", ")}] sessions=[${result.sessionLocations?.join(", ")}]`,
    ).toBe(true);
  }

  /** Dockview tab for the PR detail panel (title starts as "Pull Request", updated to "PR #N"). */
  prDetailTab(): Locator {
    return this.page.locator(".dv-default-tab").filter({ hasText: /^(Pull Request|PR #\d+)$/ });
  }

  /** Click a dockview tab by its visible label (e.g. "Changes", "Files", "Terminal"). */
  async clickTab(label: string): Promise<void> {
    const tab = this.page.locator(`.dv-default-tab:has-text('${label}')`);
    await tab.click();
  }

  /**
   * Click the session/chat tab regardless of its current title.
   * Session tabs are renamed from "Agent" to "#N AgentName" by useChatSessionTitle,
   * so this uses the stable data-testid on the ContextMenuTrigger instead.
   */
  async clickSessionChatTab(): Promise<void> {
    await this.clickFirstAvailableSessionTab();
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
    await expect
      .poll(
        async () => {
          if ((await toggle.getAttribute("aria-expanded")) === "true") return true;
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
    const editor = await this.waitForEditableChatEditor();
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
    const editor = await this.waitForEditableChatEditor();
    await editor.click();
    await editor.fill(text);
    await this.activeChat().getByTestId("submit-message-button").click();
  }

  /**
   * Wait for the agent reply containing `text` at the given 0-based match
   * `index` to be visible after a follow-up prompt. On first timeout, reload
   * once so SSR re-fetches the persisted turn, then re-assert.
   *
   * This rides out the same WS-subscribe race `waitForChatIdle` handles, but
   * for the reply message itself: a mid-session prompt's response event can be
   * dropped when the client's WS subscription loses the race with the agent's
   * reply (common after repeated restart/resume cycles). The reply is persisted
   * server-side, so a single reload recovers it.
   */
  async expectChatResponseVisible(text: string, index = 0, opts: { timeout?: number } = {}) {
    const timeout = opts.timeout ?? 30_000;
    const target = () => this.chat.getByText(text, { exact: false }).nth(index);
    try {
      await expect(target()).toBeVisible({ timeout });
    } catch {
      await this.page.reload();
      await this.waitForLoad();
      await expect(target()).toBeVisible({ timeout });
    }
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
  async expectTerminalConnected(timeout = 15_000): Promise<void> {
    await this.terminal.getByTestId("passthrough-loading").waitFor({ state: "hidden", timeout });
  }

  /**
   * Wait for the terminal shell to be connected (buffer has content from
   * the prompt), then type a command and press Enter.
   */
  async typeInTerminal(command: string): Promise<void> {
    await expect
      .poll(async () => (await this.readXtermBuffer("terminal-panel")).length > 0, {
        timeout: 15_000,
        message: "Waiting for terminal shell to connect",
      })
      .toBe(true);

    const xterm = this.terminal.locator(".xterm");
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

  /** "New Session" menu item in the dockview + dropdown. */
  newSessionMenuButton(): Locator {
    return this.page.getByTestId("new-session-button");
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
    return this.sessionLaunchDialog().locator("textarea");
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
    return this.sessionTabTriggers().filter({ hasText: text });
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
    const tab = this.sessionTabByText(text);
    await tab.click({ button: "right" });
  }

  /** Right-click the first session tab (useful when there is only one session). */
  async rightClickFirstSessionTab(): Promise<void> {
    const tab = this.sessionTabTriggers().first();
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
    return this.files.locator(`[data-testid="file-tree-node"][data-path="${nodePath}"]`);
  }

  /** All file tree nodes with data-selected="true". */
  fileTreeSelectedNodes(): Locator {
    return this.files.locator("[data-selected='true']");
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
    if (await this.revisionsPopover().isVisible()) return;
    await this.rewindButton().click();
    await expect(this.revisionsPopover()).toBeVisible({ timeout: 5_000 });
  }

  /** Open rewind, click revert on the row with the given revision number, and confirm. */
  async revertToRevision(n: number): Promise<void> {
    await this.openRewind();
    await this.revertButton(this.revisionRow(n)).click();
    await expect(this.revertConfirmDialog()).toBeVisible({ timeout: 5_000 });
    await this.revertConfirmOk().click();
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
