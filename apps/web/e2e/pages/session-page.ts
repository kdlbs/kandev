import { type Locator, type Page } from "@playwright/test";

export class SessionPage {
  readonly chat: Locator;
  readonly sidebar: Locator;
  readonly terminal: Locator;
  readonly files: Locator;
  readonly planPanel: Locator;
  readonly stepper: Locator;

  constructor(private readonly page: Page) {
    this.chat = page.getByTestId("session-chat");
    this.sidebar = page.getByTestId("task-sidebar");
    this.terminal = page.getByTestId("terminal-panel");
    this.files = page.getByTestId("files-panel");
    this.planPanel = page.getByTestId("plan-panel");
    this.stepper = page.getByTestId("workflow-stepper");
  }

  async waitForLoad(timeout = 15_000) {
    await this.chat.waitFor({ state: "visible", timeout });
  }

  /** Scoped to the sidebar — finds task title text rendered by TaskItem. */
  taskInSidebar(title: string): Locator {
    return this.sidebar.getByText(title, { exact: false });
  }

  /** Agent STARTING or RUNNING status indicator. */
  agentStatus(): Locator {
    return this.page.getByRole("status", { name: /Agent is (starting|running)/ });
  }

  /** Divider that appears after the "New session started" status message is rendered. */
  turnComplete(): Locator {
    return this.page.getByTestId("agent-turn-complete");
  }

  /** Chat input placeholder used in plan mode. */
  planModeInput(): Locator {
    return this.page.getByPlaceholder("Continue working on the plan...");
  }

  /**
   * "Plan mode" badge shown on a message that was sent with plan mode active.
   * Appears when message.metadata.plan_mode = true, which the backend sets when
   * a session is auto-started via the enable_plan_mode workflow event.
   */
  planModeBadge(): Locator {
    return this.chat.getByText("Plan mode", { exact: true });
  }

  stepperStep(name: string): Locator {
    return this.page.getByTestId(`workflow-step-${name}`);
  }

  /**
   * Types a message into the TipTap chat input and sends it.
   * Default submit key is Cmd+Enter (chatSubmitKey = "cmd_enter").
   */
  async sendMessage(text: string) {
    const editor = this.page.locator(".tiptap.ProseMirror").first();
    await editor.click();
    await editor.fill(text);
    await editor.press("Meta+Enter");
  }
}
