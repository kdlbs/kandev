import { test, expect } from "../../fixtures/test-base";
import { expectElementsNotToIntersect } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import type { MessageQueueSettingsValue } from "../../../lib/types/system";
import {
  MESSAGE_QUEUE_SETTINGS_PATH,
  requestMessageQueueSettings,
  restoreMessageQueueSettings,
} from "../../helpers/message-queue-settings";

let baseline: MessageQueueSettingsValue | undefined;

test.beforeEach(async ({ apiClient }) => {
  baseline = (await requestMessageQueueSettings(apiClient, "GET")).settings;
});

test.afterEach(async ({ apiClient }) => {
  if (!baseline) return;
  await restoreMessageQueueSettings(apiClient, baseline);
  baseline = undefined;
});

test("mobile navigation reaches the Message Queue section with touch-safe shared settings layout", async ({
  testPage,
}) => {
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileMenuButton.click();
  const homeMenu = testPage.getByTestId("mobile-home-menu-card");
  await homeMenu.getByRole("link", { name: "Settings" }).click();
  // Settings lands on the /settings index; the queue lives on Task behavior.
  const index = testPage.getByTestId("settings-index");
  await index.getByRole("link", { name: /^Task Behavior/ }).click();

  await expect(testPage).toHaveURL(
    (url) => new URL(url).pathname === "/settings/preferences/task-behavior",
  );
  await expect(testPage.getByText("Message Queue").first()).toBeVisible();

  const input = testPage.getByTestId("message-queue-max-per-session");
  await expect(input).toBeVisible();
  const inputBox = await input.boundingBox();
  expect(inputBox).not.toBeNull();
  expect(inputBox!.height).toBeGreaterThanOrEqual(44);

  await expect
    .poll(() => testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
    .toBe(true);
  await expect(testPage.getByTestId("settings-scroll-container")).toHaveCSS("overflow-y", "auto");
  const nestedScrollOwners = await testPage.getByTestId("message-queue-settings").evaluate(
    (root) =>
      Array.from(root.querySelectorAll("*")).filter((element) => {
        const overflow = getComputedStyle(element).overflowY;
        return overflow === "auto" || overflow === "scroll";
      }).length,
  );
  expect(nestedScrollOwners).toBe(0);

  if (!baseline) throw new Error("message queue settings baseline was not captured");
  const toggle = testPage.getByTestId("message-queue-auto-merge-enabled");
  const touchTarget = testPage.getByTestId("message-queue-auto-merge-touch-target");
  await expect(toggle).toHaveAttribute("aria-checked", String(baseline.auto_merge_enabled));
  const touchBox = await touchTarget.boundingBox();
  expect(touchBox).not.toBeNull();
  expect(touchBox!.width).toBeGreaterThanOrEqual(44);
  expect(touchBox!.height).toBeGreaterThanOrEqual(44);

  await toggle.tap();
  const saveBar = testPage.getByTestId("settings-floating-save");
  await expect(saveBar).toBeVisible();
  await expectElementsNotToIntersect(touchTarget, saveBar);
  await saveBar.getByRole("button", { name: "Reset" }).tap();
  await expect(toggle).toHaveAttribute("aria-checked", String(baseline.auto_merge_enabled));
  await expect(saveBar).not.toBeVisible();

  const expected = !baseline.auto_merge_enabled;
  const saveResponse = testPage.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" &&
      new URL(response.url()).pathname === MESSAGE_QUEUE_SETTINGS_PATH,
  );
  await toggle.tap();
  await saveBar.getByRole("button", { name: "Save changes" }).tap();
  expect((await saveResponse).request().postDataJSON()).toEqual({ auto_merge_enabled: expected });
  await testPage.reload();
  await expect(testPage.getByTestId("message-queue-auto-merge-enabled")).toHaveAttribute(
    "aria-checked",
    String(expected),
  );
});

test("mobile configuration lock keeps the source and accessible controls consistent", async ({
  testPage,
}) => {
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
          max_per_session: 44,
          source: "configuration",
          locked: true,
          merge_enabled: true,
          auto_merge_enabled: true,
        },
      }),
    });
  });

  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileMenuButton.click();
  await testPage
    .getByTestId("mobile-home-menu-card")
    .getByRole("link", { name: "Settings" })
    .click();
  await testPage
    .getByTestId("settings-index")
    .getByRole("link", { name: /^Task Behavior/ })
    .click();

  const input = testPage.getByLabel("Maximum messages per session");
  await expect(input).toBeDisabled();
  await expect(testPage.getByTestId("message-queue-source")).toHaveText("Configuration");
  await expect(testPage.getByText(/Managed by configuration/)).toBeVisible();
  await expect(testPage.getByText(/KANDEV_QUEUE_MAX_PER_SESSION/)).toHaveCount(0);
  const inputBox = await input.boundingBox();
  expect(inputBox).not.toBeNull();
  expect(inputBox!.height).toBeGreaterThanOrEqual(44);
});
