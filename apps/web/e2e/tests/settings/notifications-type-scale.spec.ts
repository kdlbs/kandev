import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

const TURN_FINISHED = "session.turn_finished";
const CLARIFICATION_REQUESTED = "session.clarification_requested";
const PROVIDER_NAMES = ["Desktop", "Team channel"];

async function seedProvider(apiClient: ApiClient, name: string): Promise<void> {
  const response = await apiClient.rawRequest("POST", "/api/v1/notification-providers", {
    name,
    type: "local",
    events: [TURN_FINISHED, CLARIFICATION_REQUESTED],
  });
  expect(response.ok).toBe(true);
}

test.describe("Notifications settings type scale", () => {
  test("renders the card body on the shared settings scale", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    for (const name of PROVIDER_NAMES) await seedProvider(apiClient, name);

    await testPage.setViewportSize({ width: 1280, height: 1100 });
    await testPage.goto("/settings/general/notifications");

    const table = testPage.getByTestId("notification-events-desktop-table");
    await expect(table).toBeVisible();
    const description = testPage.getByText(
      "Configure external providers for remote notifications.",
    );
    await expect(description).toBeVisible();

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-desktop", {
      caption: "Settings → General → Notifications at 1280px",
    });

    // The page body inherits the Card base (text-xs/relaxed = 12px); group
    // headings sit one step up at text-sm (14px). Assert the computed sizes so
    // the capture is backed by a real measurement, not just an eyeball.
    expect(
      await description.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
    ).toBeCloseTo(12, 1);
    expect(await table.evaluate((el) => parseFloat(getComputedStyle(el).fontSize))).toBeCloseTo(
      12,
      1,
    );
  });
});
