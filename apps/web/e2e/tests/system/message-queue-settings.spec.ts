import { test, expect } from "../../fixtures/test-base";
import type {
  MessageQueueSettingsResponse,
  MessageQueueSettingsValue,
} from "../../../lib/types/system";
import {
  MESSAGE_QUEUE_SETTINGS_PATH,
  requestMessageQueueSettings,
  restoreMessageQueueSettings,
} from "../../helpers/message-queue-settings";

test.describe.serial("Message Queue task behavior settings", () => {
  let baseline: MessageQueueSettingsValue | undefined;

  test.beforeEach(async ({ apiClient }) => {
    baseline = (await requestMessageQueueSettings(apiClient, "GET")).settings;
  });

  test.afterEach(async ({ apiClient }) => {
    if (!baseline) return;
    await restoreMessageQueueSettings(apiClient, baseline);
    baseline = undefined;
  });

  test("admin navigates to Task behavior, saves, and reloads the live setting", async ({
    testPage,
  }) => {
    if (!baseline) throw new Error("message queue settings baseline was not captured");
    const updated = baseline.max_per_session === 17 ? 18 : 17;
    await testPage.goto("/settings/preferences/appearance");
    const settingsNav = testPage.getByTestId("app-sidebar-settings-mode");
    await settingsNav.getByRole("link", { name: /^Task Behavior/ }).click();

    await expect(testPage).toHaveURL(
      (url) => new URL(url).pathname === "/settings/preferences/task-behavior",
    );
    await expect(testPage.getByText("Message Queue").first()).toBeVisible();

    // The Message Queue box spans the full settings column like every other
    // settings box; it must not be width-restricted.
    const queueCard = testPage.getByTestId("message-queue-settings");
    // Any other unconditional card on this page works as the reference width;
    // this used to be the Voice Mode card, which moved out to the Voice plugin.
    const siblingCard = testPage.getByTestId("archive-confirmation-card");
    await expect(queueCard).toBeVisible();
    await expect(siblingCard).toBeVisible();
    expect((await queueCard.boundingBox())?.width ?? 0).toBeCloseTo(
      (await siblingCard.boundingBox())?.width ?? 0,
      0,
    );

    const input = testPage.getByTestId("message-queue-max-per-session");
    await expect(input).toHaveValue(String(baseline.max_per_session));
    await input.fill(String(updated));
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();

    const saveResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === MESSAGE_QUEUE_SETTINGS_PATH,
    );
    await testPage.getByRole("button", { name: "Save changes" }).click();
    expect((await saveResponse).status()).toBe(200);
    await expect(testPage.getByTestId("message-queue-effective-value")).toHaveText(String(updated));
    await expect(testPage.getByTestId("message-queue-source")).toHaveText("Saved setting");

    await testPage.reload();
    await expect(input).toHaveValue(String(updated));
    await expect(testPage.getByTestId("message-queue-effective-value")).toHaveText(String(updated));
  });

  test("admin toggles queued message merging without touching the session limit", async ({
    testPage,
  }) => {
    if (!baseline) throw new Error("message queue settings baseline was not captured");
    await testPage.goto("/settings/general/message-queue");
    const toggle = testPage.getByTestId("message-queue-merge-enabled");
    await expect(toggle).toBeVisible();
    const wasEnabled = baseline.merge_enabled;
    await expect(toggle).toHaveAttribute("aria-checked", String(wasEnabled));
    await expect(
      testPage.getByText(/Only adjacent messages from the same sender can be merged/),
    ).toBeVisible();
    await expect(
      testPage.getByText(/combined, deduplicated entity references would exceed 100/),
    ).toBeVisible();

    const saveResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === MESSAGE_QUEUE_SETTINGS_PATH,
    );
    await toggle.click();
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
    await testPage.getByRole("button", { name: "Save changes" }).click();
    const response = await saveResponse;
    expect(response.status()).toBe(200);
    expect(response.request().postDataJSON()).toEqual({ merge_enabled: !wasEnabled });
    await expect(toggle).toHaveAttribute("aria-checked", String(!wasEnabled));

    await testPage.reload();
    await expect(testPage.getByTestId("message-queue-merge-enabled")).toHaveAttribute(
      "aria-checked",
      String(!wasEnabled),
    );
  });

  test("automatic merge defaults on and saves off then on without changing other fields", async ({
    testPage,
  }) => {
    if (!baseline) throw new Error("message queue settings baseline was not captured");
    expect(baseline.auto_merge_enabled).toBe(true);
    await testPage.goto("/settings/preferences/task-behavior");
    const toggle = testPage.getByTestId("message-queue-auto-merge-enabled");
    await expect(toggle).toHaveAttribute("aria-checked", "true");

    for (const expected of [false, true]) {
      const saveResponse = testPage.waitForResponse(
        (response) =>
          response.request().method() === "PATCH" &&
          new URL(response.url()).pathname === MESSAGE_QUEUE_SETTINGS_PATH,
      );
      await toggle.click();
      await testPage.getByRole("button", { name: "Save changes" }).click();
      const response = await saveResponse;
      expect(response.status()).toBe(200);
      expect(response.request().postDataJSON()).toEqual({ auto_merge_enabled: expected });
      const saved = (await response.json()) as MessageQueueSettingsResponse;
      expect(saved.settings).toMatchObject({
        max_per_session: baseline.max_per_session,
        merge_enabled: baseline.merge_enabled,
        auto_merge_enabled: expected,
      });
      await testPage.reload();
      await expect(testPage.getByTestId("message-queue-auto-merge-enabled")).toHaveAttribute(
        "aria-checked",
        String(expected),
      );
    }
  });

  test("environment override is visible and read-only", async ({ testPage }) => {
    await testPage.route(`**${MESSAGE_QUEUE_SETTINGS_PATH}`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: {
            max_per_session: baseline?.max_per_session ?? 10,
            merge_enabled: true,
            auto_merge_enabled: true,
          },
          effective: {
            max_per_session: 41,
            source: "environment",
            locked: true,
            merge_enabled: true,
            auto_merge_enabled: true,
          },
        } satisfies MessageQueueSettingsResponse),
      });
    });

    await testPage.goto("/settings/preferences/task-behavior");
    await expect(testPage.getByTestId("message-queue-max-per-session")).toBeDisabled();
    await expect(testPage.getByTestId("message-queue-effective-value")).toHaveText("41");
    await expect(testPage.getByTestId("message-queue-source")).toHaveText("Environment");
    await expect(testPage.getByText(/KANDEV_QUEUE_MAX_PER_SESSION/).first()).toBeVisible();
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
    // The environment lock only governs max_per_session; both merge settings stay editable.
    await expect(testPage.getByTestId("message-queue-merge-enabled")).toBeEnabled();
    await expect(testPage.getByTestId("message-queue-auto-merge-enabled")).toBeEnabled();
  });

  test("configuration override is visible and read-only", async ({ testPage }) => {
    await testPage.route(`**${MESSAGE_QUEUE_SETTINGS_PATH}`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: {
            max_per_session: baseline?.max_per_session ?? 10,
            merge_enabled: true,
            auto_merge_enabled: true,
          },
          effective: {
            max_per_session: 43,
            source: "configuration",
            locked: true,
            merge_enabled: true,
            auto_merge_enabled: true,
          },
        } satisfies MessageQueueSettingsResponse),
      });
    });

    await testPage.goto("/settings/preferences/task-behavior");
    await expect(testPage.getByLabel("Maximum messages per session")).toBeDisabled();
    await expect(testPage.getByTestId("message-queue-effective-value")).toHaveText("43");
    await expect(testPage.getByTestId("message-queue-source")).toHaveText("Configuration");
    await expect(testPage.getByText(/Managed by configuration/)).toBeVisible();
    await expect(testPage.getByText(/KANDEV_QUEUE_MAX_PER_SESSION/)).toHaveCount(0);
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
    await expect(testPage.getByTestId("message-queue-merge-enabled")).toBeEnabled();
    await expect(testPage.getByTestId("message-queue-auto-merge-enabled")).toBeEnabled();
  });

  test("member can navigate to the setting but cannot edit it", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/appearance");
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
    await settingsNav.getByRole("link", { name: /^Task Behavior/ }).click();

    await expect(testPage.getByTestId("message-queue-max-per-session")).toBeDisabled();
    await expect(testPage.getByTestId("message-queue-merge-enabled")).toBeDisabled();
    await expect(testPage.getByTestId("message-queue-auto-merge-enabled")).toBeDisabled();
    await expect(testPage.getByText("Only administrators can change this setting.")).toBeVisible();
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
  });
});
