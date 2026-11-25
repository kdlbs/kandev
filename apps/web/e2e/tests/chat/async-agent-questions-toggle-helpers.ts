import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";

export const ASYNC_QUESTIONS_FLAG = "features.asyncQuestions";
export const ASYNC_QUESTIONS_ENV = "KANDEV_FEATURES_ASYNC_QUESTIONS";

export type AsyncQuestionsFlag = {
  key: string;
  effective_value: boolean;
  default_value: boolean;
  override_value: boolean | null;
  source: string;
  env_locked: boolean;
  requires_restart_to_apply: boolean;
};

export async function readAsyncQuestionsFlag(
  page: Page,
  backend: BackendContext,
): Promise<AsyncQuestionsFlag> {
  const response = await page.request.get(`${backend.baseUrl}/api/v1/runtime-flags`);
  expect(response.ok()).toBe(true);
  const payload = (await response.json()) as { flags: AsyncQuestionsFlag[] };
  const flag = payload.flags.find((candidate) => candidate.key === ASYNC_QUESTIONS_FLAG);
  expect(flag, "the runtime flag catalog must contain async agent questions").toBeDefined();
  return flag!;
}

export async function openAsyncQuestionsToggle(page: Page) {
  await page.goto("/settings/system/feature-toggles");
  const settings = page.getByTestId("feature-toggles-settings");
  await expect(settings).toBeVisible({ timeout: 15_000 });
  const card = page.getByTestId(`feature-toggle-${ASYNC_QUESTIONS_FLAG}`);
  await expect(card).toBeVisible();
  await card.scrollIntoViewIfNeeded();
  return { settings, card, toggle: card.getByRole("switch") };
}

export async function saveAsyncQuestionsOverride(page: Page, enabled: boolean): Promise<void> {
  const { card, toggle } = await openAsyncQuestionsToggle(page);
  await expect(toggle).toHaveAttribute("aria-checked", String(!enabled));
  await toggle.click();
  await expect(card).toHaveAttribute("data-settings-dirty", "true");
  const floatingSave = page.getByTestId("settings-floating-save");
  await floatingSave.getByRole("button", { name: "Save changes" }).click();
  await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
  await expect(toggle).toHaveAttribute("aria-checked", String(enabled));
}

export async function clearAsyncQuestionsOverride(
  page: Page,
  backend: BackendContext,
): Promise<void> {
  await backend.ensureReady();
  const response = await page.request.patch(
    `${backend.baseUrl}/api/v1/runtime-flags/${encodeURIComponent(ASYNC_QUESTIONS_FLAG)}`,
    { data: { override: null } },
  );
  expect(response.ok()).toBe(true);
  await backend.restart();
}

export async function expectBootAsyncQuestions(page: Page, enabled: boolean): Promise<void> {
  await page.waitForFunction((expected) => {
    const boot = (
      window as Window & {
        __KANDEV_BOOT_PAYLOAD__?: {
          initialState?: { features?: Record<string, unknown> };
        };
      }
    ).__KANDEV_BOOT_PAYLOAD__;
    return boot?.initialState?.features?.asyncQuestions === expected;
  }, enabled);
}
