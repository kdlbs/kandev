import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

const TURN_FINISHED = "session.turn_finished";
const CLARIFICATION_REQUESTED = "session.clarification_requested";
const UPDATE_AVAILABLE = "system.update_available";
const PROVIDER_NAMES = [
  "E2E mobile semantic notifications alpha",
  "E2E mobile semantic notifications beta",
  "E2E mobile semantic notifications gamma",
];

type SeededProvider = {
  id: string;
};

async function seedNotificationProvider(
  apiClient: ApiClient,
  name: string,
): Promise<SeededProvider> {
  const response = await apiClient.rawRequest("POST", "/api/v1/notification-providers", {
    name,
    type: "local",
    events: [TURN_FINISHED, CLARIFICATION_REQUESTED],
  });
  expect(response.ok).toBe(true);
  return (await response.json()) as SeededProvider;
}

async function expectViewportContained(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  const viewportWidth = await locator.page().evaluate(() => window.innerWidth);
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewportWidth);
}

test.describe("Mobile notification event settings", () => {
  test("shows semantic event rows, including updates, as touch-operable without horizontal overflow", async ({
    testPage,
    apiClient,
  }) => {
    const providers = await Promise.all(
      PROVIDER_NAMES.map((name) => seedNotificationProvider(apiClient, name)),
    );

    try {
      await testPage.goto("/settings/general/notifications");

      const eventContainer = testPage.getByTestId("notification-events-mobile-list");
      const turnFinished = testPage.getByRole("checkbox", {
        name: `Agent turn finished for ${PROVIDER_NAMES[0]}`,
      });
      const needsAnswer = testPage.getByRole("checkbox", {
        name: `Agent needs an answer for ${PROVIDER_NAMES[0]}`,
      });
      const updateAvailable = testPage.getByRole("checkbox", {
        name: `Kandev update available for ${PROVIDER_NAMES[0]}`,
      });
      const turnFinishedTarget = testPage.getByTestId(
        `notification-event-toggle-${TURN_FINISHED}-${providers[0].id}`,
      );
      const needsAnswerTarget = testPage.getByTestId(
        `notification-event-toggle-${CLARIFICATION_REQUESTED}-${providers[0].id}`,
      );
      const updateAvailableTarget = testPage.getByTestId(
        `notification-event-toggle-${UPDATE_AVAILABLE}-${providers[0].id}`,
      );
      await expect(eventContainer).toBeVisible();
      await expect(
        eventContainer.getByText(PROVIDER_NAMES[2], { exact: true }).first(),
      ).toBeVisible();
      await expect(eventContainer.getByText("Agent turn finished", { exact: true })).toBeVisible();
      await expect(
        eventContainer.getByText("Notify after each completed agent turn."),
      ).toBeVisible();
      await expect(
        eventContainer.getByText("Agent needs an answer", { exact: true }),
      ).toBeVisible();
      await expect(
        eventContainer.getByText("Notify when the agent explicitly asks you a question."),
      ).toBeVisible();
      await expect(turnFinished).toBeVisible();
      await expect(needsAnswer).toBeVisible();
      await expect(updateAvailable).not.toBeChecked();
      await expect(
        eventContainer.getByText("Kandev update available", { exact: true }),
      ).toBeVisible();
      await expect(
        eventContainer.getByText("Notify when a newer Kandev release is available."),
      ).toBeVisible();

      await expectViewportContained(
        eventContainer.getByText("Agent turn finished", { exact: true }),
      );
      await expectViewportContained(
        eventContainer.getByText("Notify after each completed agent turn."),
      );
      await expectViewportContained(
        eventContainer.getByText("Agent needs an answer", { exact: true }),
      );
      await expectViewportContained(
        eventContainer.getByText("Notify when the agent explicitly asks you a question."),
      );
      await expectViewportContained(
        eventContainer.getByText("Kandev update available", { exact: true }),
      );
      await expectViewportContained(
        eventContainer.getByText("Notify when a newer Kandev release is available."),
      );
      await expectViewportContained(turnFinished);
      await expectViewportContained(needsAnswer);
      await expectViewportContained(updateAvailable);
      for (const target of [turnFinishedTarget, needsAnswerTarget, updateAvailableTarget]) {
        const box = await target.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.width).toBeGreaterThanOrEqual(44);
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }
      expect(
        await eventContainer.evaluate((element) => element.scrollWidth <= element.clientWidth),
      ).toBe(true);

      await updateAvailableTarget.scrollIntoViewIfNeeded();
      await updateAvailableTarget.tap();
      await expect(updateAvailable).toBeChecked();
      await testPage.getByRole("button", { name: "Save changes" }).click();
      await expect(testPage.getByRole("button", { name: "Save changes" })).not.toBeVisible();
      await testPage.reload();
      await expect(updateAvailable).toBeChecked();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);
    } finally {
      await Promise.all(
        providers.map((provider) =>
          apiClient.rawRequest("DELETE", `/api/v1/notification-providers/${provider.id}`),
        ),
      );
    }
  });
});
