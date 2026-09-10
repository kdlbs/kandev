import type { Page } from "@playwright/test";

export const APPRISE_EVENT = "session.turn_finished";
export const APPRISE_PROVIDER_NAME = "E2E local notifications";

type RescanOutcome = boolean | "error";

function providerResponse(appriseAvailable: boolean) {
  return {
    providers: [
      {
        id: "e2e-local-notification-provider",
        name: APPRISE_PROVIDER_NAME,
        type: "local",
        config: {},
        enabled: true,
        events: [],
        created_at: "2026-09-09T00:00:00Z",
        updated_at: "2026-09-09T00:00:00Z",
      },
    ],
    apprise_available: appriseAvailable,
    events: [APPRISE_EVENT],
  };
}

export async function routeAppriseRescans(
  page: Page,
  outcomes: RescanOutcome[],
): Promise<{ startRescan: () => void }> {
  let rescanStarted = false;
  let outcomeIndex = 0;
  await page.route("**/api/v1/notification-providers**", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    if (!rescanStarted) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        json: providerResponse(false),
      });
      return;
    }

    const outcome = outcomes[Math.min(outcomeIndex++, Math.max(outcomes.length - 1, 0))] ?? false;
    if (outcome === "error") {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        json: { error: "provider list unavailable" },
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      json: providerResponse(outcome),
    });
  });
  return {
    startRescan: () => {
      rescanStarted = true;
    },
  };
}
