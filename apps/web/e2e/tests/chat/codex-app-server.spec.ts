import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { openCompletedNativeForkTask } from "./conversation-fork-helpers";

test("confirms a native conversation fork in the desktop dialog", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(60_000);
  const { taskId, sessionId } = await openCompletedNativeForkTask(
    testPage,
    apiClient,
    seedData,
    "Codex app-server conversation fork",
  );

  const trigger = testPage.getByTestId("fork-conversation-trigger");
  await expect(trigger).toBeVisible();
  await trigger.click();
  const dialog = testPage.getByRole("dialog", { name: "Fork conversation" });
  await expect(dialog).toContainText("Files remain shared in this workspace");
  await waitForFiniteAnimations(dialog);
  await prCapture.screenshot("codex-conversation-fork-desktop", {
    caption: "Fork through a completed Codex conversation while files remain shared",
  });
  await dialog.getByTestId("fork-conversation-confirm").click();

  await expect
    .poll(() =>
      testPage.evaluate(() => {
        const store = (
          window as Window & {
            __KANDEV_E2E_STORE__?: {
              getState: () => { tasks: { activeSessionId: string | null } };
            };
          }
        ).__KANDEV_E2E_STORE__;
        return store?.getState().tasks.activeSessionId ?? null;
      }),
    )
    .toBe("e2e-fork-session");
  expect(taskId).toBeTruthy();
  expect(sessionId).toBeTruthy();
});
