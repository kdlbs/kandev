import { type Locator, type Page, expect } from "@playwright/test";

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
  todoIndicator() {
    return this.page.getByTestId("todo-indicator");
  }

  async waitForLoad(timeout = 15_000) {
    // When multiple session tabs are open, multiple session-chat panels exist in
    // the DOM but only the active one is visible. Use :visible to avoid matching
    // a hidden background panel (which would cause the wait to time out).
    await this.page
      .locator("[data-testid='session-chat']:visible")
      .first()
      .waitFor({ state: "visible", timeout });
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

  /** Sidebar section header (e.g. "Turn Finished", "Running", "Backlog"). */
  sidebarSection(label: string): Locator {
    return this.sidebar.getByTestId(`sidebar-section-${label}`);
  }

  /** Task title scoped to a specific sidebar section (Turn Finished, Running, Backlog). */
  taskInSection(title: string, sectionLabel: string): Locator {
    return this.sidebar
      .getByTestId(`sidebar-section-${sectionLabel}`)
      .locator("..")
      .getByText(title, { exact: false });
  }

  /** Agent STARTING or RUNNING status indicator. */
  agentStatus(): Locator {
    return this.page.getByRole("status", { name: /Agent is (starting|running)/ });
  }

  /** Divider that appears after the "New session started" status message is rendered. */
  turnComplete(): Locator {
    return this.page.getByTestId("agent-turn-complete");
  }

  /** Chat input placeholder when agent is idle (default mode). */
  idleInput(): Locator {
    return this.page.locator('[data-placeholder="Continue working on the task..."]');
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
    return this.page.getByTestId("clarification-overlay");
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

  /** Custom text input on the clarification overlay. */
  clarificationInput(): Locator {
    return this.page.getByTestId("clarification-input");
  }

  /** Deferred notice shown when agent has disconnected from clarification. */
  clarificationDeferredNotice(): Locator {
    return this.page.getByTestId("clarification-deferred-notice");
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

  /** Context reset divider shown in chat after resetting agent context. */
  contextResetDivider(): Locator {
    return this.chat.getByText("Context reset");
  }

  /**
   * Delete a task via the sidebar context menu.
   * Hovers to reveal the menu trigger, opens it, and clicks "Delete".
   */
  async deleteTaskInSidebar(title: string): Promise<void> {
    await this.openSidebarMenuAndClick(title, "Delete");
  }

  /**
   * Archive a task via the sidebar context menu.
   * Hovers to reveal the menu trigger, opens it, and clicks "Archive".
   */
  async archiveTaskInSidebar(title: string): Promise<void> {
    await this.openSidebarMenuAndClick(title, "Archive");
  }

  /**
   * Open a sidebar task's dropdown menu and click an item.
   * Retries the full open-click sequence if the menu gets detached by a
   * React re-render (e.g. WS-driven sidebar update) between open and click.
   */
  private async openSidebarMenuAndClick(
    title: string,
    itemName: string,
    retries = 3,
  ): Promise<void> {
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

  /** VCS split button primary action. Pass action to match a specific state. */
  vcsPrimaryButton(action?: "commit" | "push" | "pr" | "rebase"): Locator {
    if (action) {
      return this.page.getByTestId(`vcs-primary-${action}`);
    }
    return this.page.locator('[data-testid^="vcs-primary-"]');
  }

  /** PR detail panel (auto-shown when task has an associated PR). */
  prDetailPanel(): Locator {
    return this.page.getByTestId("pr-detail-panel");
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
    await this.page.locator('[data-testid^="session-tab-"]').first().click();
  }

  /** PR files section within the changes panel. */
  prFilesSection(): Locator {
    return this.changes.getByTestId("pr-files-section");
  }

  /** Commits section within the changes panel (unified list of pushed + unpushed commits). */
  commitsSection(): Locator {
    return this.changes.getByTestId("commits-section");
  }

  /**
   * Types a message into the TipTap chat input and sends it.
   * Default submit key is Cmd+Enter (chatSubmitKey = "cmd_enter").
   * TipTap maps "Mod" to Meta on macOS and Control on Linux/Windows.
   */
  async sendMessage(text: string) {
    const editor = this.page.locator(".tiptap.ProseMirror").first();
    await editor.click();
    await editor.fill(text);
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);
  }

  /** Toggle plan mode on/off by clicking the plan mode toggle button in the toolbar. */
  async togglePlanMode() {
    const btn = this.page.getByTestId("plan-mode-toggle-button");
    await expect(btn).toBeVisible({ timeout: 10_000 });
    await btn.click();
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

  /** The new session dialog container. */
  newSessionDialog(): Locator {
    return this.page.getByRole("dialog").filter({ hasText: "New agent in" });
  }

  /** Prompt textarea inside the new session dialog. */
  newSessionPromptInput(): Locator {
    return this.newSessionDialog().locator("textarea");
  }

  /** Start Agent button inside the new session dialog. */
  newSessionStartButton(): Locator {
    return this.newSessionDialog().getByRole("button", { name: "Start Agent" });
  }

  /** Environment info badges inside the new session dialog. */
  newSessionEnvironmentInfo(): Locator {
    return this.newSessionDialog().getByText("Same environment as current session");
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
    return this.page.locator("[data-testid^='reopen-session-']");
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

  /** Context menu on a dockview tab — right-click the tab to trigger it. */
  async rightClickTab(text: string): Promise<void> {
    const tab = this.page.locator(`[data-testid^='session-tab-']:has-text('${text}')`);
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

  /** Primary star icon inside a dockview tab. */
  primaryStarInTab(text: string): Locator {
    const tab = this.page.locator(`.dv-default-tab:has-text('${text}')`);
    return tab.locator(".tabler-icon-star").first();
  }

  /** Click a task in the sidebar by title. */
  async clickTaskInSidebar(title: string): Promise<void> {
    const taskRow = this.sidebar.locator("[role='button']").filter({ hasText: title });
    await taskRow.click();
  }
}
