import { test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { assertComposerFocusAfterSend } from "./composer-focus-after-send-helpers";

// The mobile Playwright project uses touch input and only matches mobile-named
// specs. Keep this regression on the real tap path, not only desktop click.
test.describe("Mobile composer focus after send", () => {
  test.describe.configure({ retries: 1 });

  test("keeps the composer focused across consecutive touch sends", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile composer focus after send",
    );
    await assertComposerFocusAfterSend(session, testPage, () => session.tapSubmitWhenReady());
  });
});
