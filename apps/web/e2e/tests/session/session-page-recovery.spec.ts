import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test("idle recovery ignores background controls and observes an in-flight resume", async ({
  testPage,
}) => {
  await testPage.setContent(`
    <div data-testid="session-chat" hidden>
      <button data-testid="recovery-resume-button">Background resume</button>
    </div>
    <div data-testid="session-chat">
      <button id="primary" data-testid="recovery-resume-button" disabled>Resuming</button>
      <button id="duplicate" data-testid="recovery-resume-button">Resume</button>
      <div id="editor" class="tiptap ProseMirror" contenteditable="true"
        data-placeholder="Continue working on the task..." hidden></div>
    </div>
  `);
  await testPage.evaluate(() => {
    document.getElementById("duplicate")!.addEventListener("click", () => {
      document.body.dataset.duplicateClicked = "true";
    });
    document.getElementById("primary")!.addEventListener("click", () => {
      document.getElementById("primary")!.remove();
      document.getElementById("duplicate")!.remove();
      document.getElementById("editor")!.hidden = false;
    });
  });
  const session = new SessionPage(testPage);
  await expect(session.recoveryResumeButton()).toHaveCount(1);
  await expect(session.recoveryResumeButton()).toBeDisabled();
  await testPage.locator("#primary").evaluate((button: HTMLButtonElement) => {
    button.disabled = false;
  });
  await session.waitForChatIdle({ timeout: 5_000, requireEditable: true });
  await expect(session.anyIdleInput()).toBeVisible();
  await expect(testPage.locator("body")).not.toHaveAttribute("data-duplicate-clicked", "true");
});
