// Regression coverage for the silent-clarification-submit-failure fix:
// a failed /respond POST must surface a retry affordance instead of a dead
// button, and a 409 caused by the bundle going inactive must never be
// reported to the user as a successful answer.
import { test, expect } from "../../fixtures/test-base";
import { activeSessionId, seedClarificationSession } from "../../helpers/clarification";
import { watchWs } from "../../helpers/causal-waits";
import { waitForSessionSettled } from "./quick-chat-helpers";

test.describe("Clarification submit failure feedback", () => {
  test("surfaces a failed submit with a retry that preserves the answer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const ws = watchWs(testPage);
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Clarification Submit Failure",
      { scenario: "clarification" },
    );
    const sessionId = await activeSessionId(testPage);
    if (!sessionId) throw new Error("expected an active session for clarification retry");

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    let attempt = 0;
    await testPage.route("**/api/v1/clarification/*/respond", async (route) => {
      attempt += 1;
      if (attempt === 1) {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({
            error: "claim active clarification bundle: context deadline exceeded",
          }),
        });
        return;
      }
      await route.continue();
    });

    const postgres = session.clarificationOption("PostgreSQL");
    await postgres.click();

    const errorBanner = testPage.getByTestId("clarification-submit-error");
    await expect(errorBanner).toBeVisible({ timeout: 15_000 });
    // The user's answer must survive the failed submit -- no re-typing on retry.
    await expect(postgres).toHaveAttribute("data-selected", "true");
    await expect(session.idleInput()).toHaveCount(0);

    const settled = waitForSessionSettled(ws, sessionId);
    await testPage.getByTestId("clarification-retry").click();

    await settled;
    await expect(errorBanner).toHaveCount(0);
    await expect(session.idleInput()).toBeVisible();
    expect(attempt).toBe(2);
  });

  test("treats an inactive-bundle 409 as expired, never as a silent success", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Clarification Expired Conflict",
      { scenario: "clarification" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    await testPage.route("**/api/v1/clarification/*/respond", async (route) => {
      await route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({
          error: "clarification request is no longer active",
          code: "not_active",
        }),
      });
    });

    await session.clarificationOption("PostgreSQL").click();

    const expiredBanner = testPage.getByTestId("clarification-expired");
    await expect(expiredBanner).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("clarification-retry")).toHaveCount(0);
    // A dropped answer must never flip the chat back to idle -- that would be
    // reporting success for an answer nobody received.
    await expect(session.idleInput()).toHaveCount(0);
    await expect(session.clarificationOverlay()).toBeVisible();
  });
});
