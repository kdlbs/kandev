import { type Locator, type Page } from "@playwright/test";

export class TaskDetailPage {
  readonly sessionChat: Locator;
  readonly turnCompleteIndicator: Locator;

  constructor(private page: Page) {
    this.sessionChat = page.getByTestId("session-chat");
    this.turnCompleteIndicator = page.getByTestId("agent-turn-complete");
  }

  async goto(taskId: string, sessionId: string) {
    await this.page.goto(`/task/${taskId}/${sessionId}`);
  }

  async waitForAgentResponse(text: string, timeoutMs = 30_000) {
    await this.sessionChat.locator(`text=${text}`).waitFor({
      state: "visible",
      timeout: timeoutMs,
    });
  }
}
