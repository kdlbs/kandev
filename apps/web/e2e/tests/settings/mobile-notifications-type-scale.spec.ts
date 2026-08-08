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

test.describe("Mobile notifications settings type scale", () => {
  test("renders the stacked event list on the shared settings scale", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    for (const name of PROVIDER_NAMES) await seedProvider(apiClient, name);

    await testPage.goto("/settings/general/notifications");

    const eventList = testPage.getByTestId("notification-events-mobile-list");
    await expect(eventList).toBeVisible();

    const providerName = eventList.getByText(PROVIDER_NAMES[0], { exact: true }).first();
    await expect(providerName).toBeVisible();

    // Capture before asserting so a run against the pre-fix code still produces
    // the "before" image for the PR description.
    await prCapture.screenshot("notifications-mobile", {
      caption: "Settings → General → Notifications on a phone viewport",
    });

    // The mobile provider rows used to be text-sm while the section heading
    // above them inherited the 12px Card base — larger children under a smaller
    // heading. Both sit at 12px now.
    expect(
      await providerName.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
    ).toBeCloseTo(12, 1);
  });
});
