import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { MessageQueueSettingsResponse } from "../../../lib/types/system";

const SETTINGS_PATH = "/api/v1/system/message-queue/settings";

async function requestSettings(
  apiClient: ApiClient,
  method: "GET" | "PATCH",
  maxPerSession?: number,
): Promise<MessageQueueSettingsResponse> {
  const response = await apiClient.rawRequest(
    method,
    SETTINGS_PATH,
    maxPerSession === undefined ? undefined : { max_per_session: maxPerSession },
  );
  if (!response.ok) {
    throw new Error(
      `${method} ${SETTINGS_PATH} failed (${response.status}): ${await response.text()}`,
    );
  }
  return response.json() as Promise<MessageQueueSettingsResponse>;
}

test.describe.serial("Message Queue general settings", () => {
  let baseline: number | undefined;

  test.beforeEach(async ({ apiClient }) => {
    baseline = (await requestSettings(apiClient, "GET")).settings.max_per_session;
  });

  test.afterEach(async ({ apiClient }) => {
    if (baseline === undefined) return;
    await requestSettings(apiClient, "PATCH", baseline);
    baseline = undefined;
  });

  test("admin navigates to Message Queue, saves, and reloads the live setting", async ({
    testPage,
  }) => {
    const updated = baseline === 17 ? 18 : 17;
    await testPage.goto("/settings/general/appearance");
    const settingsNav = testPage.getByTestId("app-sidebar-settings-mode");
    await settingsNav.getByRole("link", { name: "Message Queue" }).click();

    await expect(testPage).toHaveURL(
      (url) => new URL(url).pathname === "/settings/general/message-queue",
    );
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Message Queue");

    const input = testPage.getByTestId("message-queue-max-per-session");
    await expect(input).toHaveValue(String(baseline));
    await input.fill(String(updated));
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();

    const saveResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === SETTINGS_PATH,
    );
    await testPage.getByRole("button", { name: "Save changes" }).click();
    expect((await saveResponse).status()).toBe(200);
    await expect(testPage.getByTestId("message-queue-effective-value")).toHaveText(String(updated));
    await expect(testPage.getByTestId("message-queue-source")).toHaveText("Saved setting");

    await testPage.reload();
    await expect(input).toHaveValue(String(updated));
    await expect(testPage.getByTestId("message-queue-effective-value")).toHaveText(String(updated));
  });

  test("environment override is visible and read-only", async ({ testPage }) => {
    await testPage.route(`**${SETTINGS_PATH}`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: { max_per_session: baseline },
          effective: { max_per_session: 41, source: "environment", locked: true },
        } satisfies MessageQueueSettingsResponse),
      });
    });

    await testPage.goto("/settings/general/message-queue");
    await expect(testPage.getByTestId("message-queue-max-per-session")).toBeDisabled();
    await expect(testPage.getByTestId("message-queue-effective-value")).toHaveText("41");
    await expect(testPage.getByTestId("message-queue-source")).toHaveText("Environment");
    await expect(testPage.getByText(/KANDEV_QUEUE_MAX_PER_SESSION/).first()).toBeVisible();
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
  });

  test("member can navigate to the setting but cannot edit it", async ({ testPage }) => {
    await testPage.goto("/settings/general/appearance");
    await testPage.waitForFunction(() => Boolean(window.__KANDEV_E2E_STORE__));
    await testPage.evaluate(() => {
      const store = window.__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.getState().setAuthState({
        mode: "enabled",
        authenticated: true,
        user: {
          id: "e2e-member",
          email: "member@e2e.dev",
          display_name: "E2E Member",
          role: "member",
          status: "active",
        },
        ssoProviders: [],
      });
    });
    const settingsNav = testPage.getByTestId("app-sidebar-settings-mode");
    await settingsNav.getByRole("link", { name: "Message Queue" }).click();

    await expect(testPage.getByTestId("message-queue-max-per-session")).toBeDisabled();
    await expect(testPage.getByText("Only administrators can change this setting.")).toBeVisible();
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
  });
});
