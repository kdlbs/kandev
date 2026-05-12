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
  todoIndicator() {
    return this.page.getByTestId("todo-indicator");
  }
  /** Resume button shown in the FailedSessionBanner. */
  failedSessionResumeButton(): Locator {
    return this.page.getByTestId("failed-session-resume-button");
  }
  /** Span wrapper around the resume button — used to trigger tooltip on disabled state. */
  failedSessionResumeWrapper(): Locator {
    return this.page.getByTestId("failed-session-resume-wrapper");
  }
  /** Cancel button shown in the chat toolbar while an agent turn is running. */
  cancelAgentButton(): Locator {
    return this.page.getByTestId("cancel-agent-button");
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

  sidebarTaskItem(title: string): Locator {
    return this.sidebar.getByTestId("sidebar-task-item").filter({
      has: this.page.getByText(title, { exact: false }),
    });
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

  /** Final "Submit answers" button (rendered only on the last step). */
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

  /** PR detail panel (auto-shown when task has an associated PR). */
  prDetailPanel(): Locator {
    return this.page.getByTestId("pr-detail-panel");
  }

  /** "Approve PR" button inside the PR detail panel header. Hidden when the
   * current GitHub user authored the PR (self-approval is rejected upstream). */
  prApproveButton(): Locator {
    return this.page.getByTestId("pr-approve-button");
  }

  // --- CI hover popover accessors (kept here so the spec stays declarative) ---

  /** The single-PR hover popover content (visible after hovering the topbar button). */
  prTopbarPopover(): Locator {
    return this.page.getByTestId("pr-topbar-popover");
  }

  /** Multi-PR aggregate popover content. */
  prTopbarPopoverAggregate(): Locator {
    return this.page.getByTestId("pr-topbar-popover-aggregate");
  }

  /** A specific bucket group inside the popover by kind. */
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
    await this.prTopbarButton().hover();
    await expect(this.prTopbarPopover()).toBeVisible({ timeout: 5_000 });
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

  /** Expand a collapsible section in the changes panel if currently collapsed. */
  async expandChangesSection(testId: string): Promise<void> {
    const toggle = this.changes.getByTestId(`${testId}-collapse-toggle`);
    await expect(toggle).toBeVisible({ timeout: 15_000 });
    if ((await toggle.getAttribute("aria-expanded")) === "false") {
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-expanded", "true");
    }
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
