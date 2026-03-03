import { type Locator, type Page, expect } from "@playwright/test";

export class SessionPage {
  readonly chat: Locator;
  readonly sidebar: Locator;
  readonly terminal: Locator;
  readonly files: Locator;
  readonly planPanel: Locator;
  readonly stepper: Locator;
  readonly passthroughTerminal: Locator;

  constructor(private readonly page: Page) {
    this.chat = page.getByTestId("session-chat");
    this.sidebar = page.getByTestId("task-sidebar");
    this.terminal = page.getByTestId("terminal-panel");
    this.files = page.getByTestId("files-panel");
    this.planPanel = page.getByTestId("plan-panel");
    this.stepper = page.getByTestId("workflow-stepper");
    this.passthroughTerminal = page.getByTestId("passthrough-terminal");
  }

  async waitForLoad(timeout = 15_000) {
    await this.chat.waitFor({ state: "visible", timeout });
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

  /** Sidebar section header (e.g. "Review", "In Progress", "Backlog"). */
  sidebarSection(label: string): Locator {
    return this.sidebar.getByTestId(`sidebar-section-${label}`);
  }

  /** Task title scoped to a specific sidebar section (Review, In Progress, Backlog). */
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

  /**
   * Delete a task via the sidebar context menu.
   * Hovers to reveal the menu trigger, opens it, and clicks "Delete".
   */
  async deleteTaskInSidebar(title: string): Promise<void> {
    const taskRow = this.sidebar.locator('[role="button"]').filter({ hasText: title });
    await taskRow.hover();
    await taskRow.getByRole("button", { name: "Task actions" }).click();
    await this.page.getByRole("menuitem", { name: "Delete" }).click();
  }

  /**
   * Archive a task via the sidebar context menu.
   * Hovers to reveal the menu trigger, opens it, and clicks "Archive".
   */
  async archiveTaskInSidebar(title: string): Promise<void> {
    const taskRow = this.sidebar.locator('[role="button"]').filter({ hasText: title });
    await taskRow.hover();
    await taskRow.getByRole("button", { name: "Task actions" }).click();
    await this.page.getByRole("menuitem", { name: "Archive" }).click();
  }

  stepperStep(name: string): Locator {
    return this.page.getByTestId(`workflow-step-${name}`);
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

  /** Toggle plan mode on/off via Shift+Tab in the TipTap editor. */
  async togglePlanMode() {
    const editor = this.page.locator(".tiptap.ProseMirror").first();
    await editor.click();
    await editor.press("Shift+Tab");
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
   * chat, terminal, files, and sidebar are all visible.
   */
  async expectDefaultLayout(): Promise<void> {
    await expect(this.chat).toBeVisible({ timeout: 10_000 });
    await expect(this.terminal).toBeVisible({ timeout: 10_000 });
    await expect(this.files).toBeVisible({ timeout: 10_000 });
    await expect(this.sidebar).toBeVisible();
  }
}
