import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

const TURN_FINISHED = "session.turn_finished";
const CLARIFICATION_REQUESTED = "session.clarification_requested";
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
  test("shows both semantic event rows as touch-operable without horizontal overflow", async ({
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
      const turnFinishedTarget = testPage.getByTestId(
        `notification-event-toggle-${TURN_FINISHED}-${providers[0].id}`,
      );
      const needsAnswerTarget = testPage.getByTestId(
        `notification-event-toggle-${CLARIFICATION_REQUESTED}-${providers[0].id}`,
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
      await expectViewportContained(turnFinished);
      await expectViewportContained(needsAnswer);
      for (const target of [turnFinishedTarget, needsAnswerTarget]) {
        const box = await target.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.width).toBeGreaterThanOrEqual(44);
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }
      expect(
        await eventContainer.evaluate((element) => element.scrollWidth <= element.clientWidth),
      ).toBe(true);

      for (const target of [turnFinishedTarget, needsAnswerTarget]) {
        await target.scrollIntoViewIfNeeded();
        await target.tap();
      }
      await expect(turnFinished).not.toBeChecked();
      await expect(needsAnswer).not.toBeChecked();
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
