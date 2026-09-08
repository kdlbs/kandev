import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";

// Regression coverage for the composer losing focus after a send. ProseMirror
// maps its `editable` state onto the DOM `contenteditable` attribute, and a
// real browser blurs the element when that attribute flips to `false` (which
// every send does for its duration) without restoring focus when it flips
// back -- jsdom cannot reproduce this, so Playwright is the only witness.
test.describe("Composer focus after send", () => {
  // Retries here absorb a pre-existing, unrelated dockview panel-portal race
  // (the chat panel's React subtree occasionally remounts a moment after
  // task load, independent of this composer's own send flow -- see the task
  // plan for reproduction). A real regression in the focus-restore wiring
  // fails on every retry, so this does not mask this spec's own assertion.
  test.describe.configure({ retries: 1 });

  test("keeps the composer focused across consecutive sends with no intervening click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Composer focus after send",
    );
    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");

    await session.sendMessageViaButton("first message after send");
    await expect(
      session.activeChat().getByText("first message after send", { exact: false }),
    ).toBeVisible({ timeout: 15_000 });
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    await expect(editor).toBeFocused({ timeout: 10_000 });

    // No click here: the editor must already hold focus from the fix, so
    // typing lands directly in the composer.
    await testPage.keyboard.type("second message after send");
    await session.clickSubmitWhenReady();
    await expect(
      session.activeChat().getByText("second message after send", { exact: false }),
    ).toBeVisible({ timeout: 15_000 });
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    await expect(editor).toBeFocused({ timeout: 10_000 });
  });
});
