import { expect, test } from "../../fixtures/test-base";

test.describe("bfcache restore reload", () => {
  test("reloads the app when the page is restored from a frozen snapshot", async ({ testPage }) => {
    await testPage.goto("/");

    // A normal load is not a restore: the document must not reload by itself.
    expect(await testPage.evaluate(() => performance.getEntriesByType("navigation")[0]?.type)).toBe(
      "navigate",
    );

    // Chrome's "Duplicate tab" (and any bfcache restore) brings the page back
    // from a frozen snapshot and fires pageshow with persisted=true. The app
    // reloads so the no-store boot payload is re-fetched instead of showing
    // the frozen state. Arm the causal wait BEFORE dispatching: the frame
    // navigation IS the cause, so we wait on it rather than budgeting the
    // effect.
    const reloaded = testPage.waitForEvent("framenavigated");
    await testPage
      .evaluate(() => {
        window.dispatchEvent(new PageTransitionEvent("pageshow", { persisted: true }));
      })
      .catch(() => undefined); // the reload may tear down the evaluate context
    await reloaded;

    // The fresh document reports the reload navigation type. Poll with the
    // default assertion timeout; the entry exists as soon as the frame
    // commits, and the catch tolerates the mid-navigation context teardown.
    await expect
      .poll(() =>
        testPage
          .evaluate(() => performance.getEntriesByType("navigation")[0]?.type)
          .catch(() => undefined),
      )
      .toBe("reload");
  });
});
