import { expect, test } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";

test("chat readiness survives an automatic recovery completing during observation", async ({
  page,
}) => {
  test.setTimeout(5_000);
  await page.setContent(`
    <section data-testid="session-chat">
      <button data-testid="recovery-resume-button">Resume</button>
      <div hidden class="tiptap ProseMirror" contenteditable="true"
        data-placeholder="Continue working on the task...">Ready</div>
    </section>
  `);
  const session = new SessionPage(page);
  const recovery = session.recoveryResumeButton();
  const isVisible = recovery.isVisible.bind(recovery);
  // Complete recovery between observing the action and interacting with it.
  recovery.isVisible = async () => {
    const visible = await isVisible();
    if (visible) {
      await recovery.evaluate((button) => {
        const input = document.querySelector<HTMLElement>("[contenteditable]")!;
        button.remove();
        input.hidden = false;
      });
    }
    return visible;
  };
  session.recoveryResumeButton = () => recovery;

  await session.waitForChatIdle({ timeout: 2_000, requireEditable: true });
  await expect(session.anyIdleInput()).toBeVisible();
  await expect(session.recoveryResumeButton()).toHaveCount(0);
});
