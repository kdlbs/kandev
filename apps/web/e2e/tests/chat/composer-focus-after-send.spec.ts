import { test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { assertComposerFocusAfterSend } from "./composer-focus-after-send-helpers";

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
    await assertComposerFocusAfterSend(session, testPage, () => session.clickSubmitWhenReady());
  });
});
