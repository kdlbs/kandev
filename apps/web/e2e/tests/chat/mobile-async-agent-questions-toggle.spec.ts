import { test, expect } from "../../fixtures/test-base";
import { seedClarificationSession } from "../../helpers/clarification";
import {
  ASYNC_QUESTIONS_ENV,
  clearAsyncQuestionsOverride,
  openAsyncQuestionsToggle,
  readAsyncQuestionsFlag,
} from "./async-agent-questions-toggle-helpers";

test.describe("Mobile async agent questions release toggle", () => {
  test("keeps blocking questions usable and shows the environment lock", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await clearAsyncQuestionsOverride(testPage, backend);
    await testPage.reload();
    const defaultFlag = await readAsyncQuestionsFlag(testPage, backend);
    expect(defaultFlag.effective_value).toBe(false);
    expect(defaultFlag.override_value).toBeNull();
    const { card, toggle } = await openAsyncQuestionsToggle(testPage);
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expect(toggle).toBeEnabled();
    await expect.poll(async () => (await card.boundingBox())?.width ?? 0).toBeGreaterThan(0);
    await prCapture.screenshot("mobile-async-questions-toggle-off", {
      caption: "Async agent questions are off by default in the phone settings card",
    });

    const envLock = await backend.useEnv({ [ASYNC_QUESTIONS_ENV]: "true" });
    try {
      await testPage.reload();
      const lockedFlag = await readAsyncQuestionsFlag(testPage, backend);
      expect(lockedFlag.effective_value).toBe(true);
      expect(lockedFlag.env_locked).toBe(true);
      const lockedToggle = testPage
        .getByTestId("feature-toggle-features.asyncQuestions")
        .getByRole("switch");
      await expect(lockedToggle).toBeDisabled();
      await expect(lockedToggle).toHaveAttribute("aria-checked", "true");
      await prCapture.screenshot("mobile-async-questions-toggle-environment-locked", {
        caption: "The phone Feature Toggles card shows the environment-controlled lock",
      });
    } finally {
      await envLock();
    }

    await testPage.reload();
    const requestUrls: string[] = [];
    testPage.on("request", (request) => {
      if (new URL(request.url()).pathname.startsWith("/api/v1/agent-questions")) {
        requestUrls.push(request.url());
      }
    });
    await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Blocking Questions With Async Off",
      { scenario: "clarification" },
    );
    await testPage.goto("/needs-you-inbox");
    const row = testPage
      .getByTestId("needs-you-inbox-row")
      .filter({ hasText: "Mobile Blocking Questions With Async Off" });
    await expect(row).toBeVisible({ timeout: 30_000 });
    await row.getByTestId("needs-you-inbox-row-toggle").tap();
    const overlay = row.getByTestId("clarification-overlay");
    await expect(overlay).toBeVisible({ timeout: 15_000 });
    await expect(overlay).toContainText("Which database");
    expect(requestUrls).toEqual([]);
    await prCapture.screenshot("mobile-blocking-question-while-async-off", {
      caption: "The phone answers a blocking clarification in the Inbox while async is off",
    });
    await overlay.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" }).tap();
    await overlay.getByTestId("clarification-custom-submit").tap();
    await expect(row).toHaveCount(0, { timeout: 30_000 });
    await expect(overlay).not.toBeVisible({ timeout: 30_000 });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await clearAsyncQuestionsOverride(testPage, backend);
  });
});
