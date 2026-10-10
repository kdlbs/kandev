import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { openCompletedNativeForkTask } from "./conversation-fork-helpers";

test("confirms a native conversation fork in the mobile drawer", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(60_000);
  await openCompletedNativeForkTask(
    testPage,
    apiClient,
    seedData,
    "Mobile Codex app-server conversation fork",
  );

  const trigger = testPage.getByTestId("fork-conversation-trigger");
  await expect(trigger).toBeVisible();
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox).not.toBeNull();
  expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
  expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
  await trigger.tap();

  const drawer = testPage.getByRole("dialog", { name: "Fork conversation" });
  await expect(drawer).toContainText("Files remain shared in this workspace");
  await waitForFiniteAnimations(drawer);
  await prCapture.screenshot("mobile-codex-conversation-fork", {
    caption: "Mobile confirmation before forking a Codex conversation",
  });
  await drawer.getByTestId("fork-conversation-confirm").tap();
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

  const hasHorizontalOverflow = await testPage.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollWidth > root.clientWidth + 1;
  });
  expect(hasHorizontalOverflow).toBe(false);
});
