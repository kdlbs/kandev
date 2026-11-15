import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import { waitForHttp } from "./causal-waits";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { requestMessageQueueSettings } from "./message-queue-settings";
import { openTaskBehaviorRuntime } from "./settings-composition";

export async function exerciseAutoSortMergeSettings(
  page: Page,
  apiClient: ApiClient,
  mobile: boolean,
) {
  await requestMessageQueueSettings(apiClient, "PATCH", {
    auto_merge_enabled: true,
    auto_sort_merge_enabled: false,
  });
  await page.goto("/settings/preferences/task-behavior");
  await openTaskBehaviorRuntime(page);
  const merge = page.getByTestId("message-queue-auto-merge-enabled");
  const sort = page.getByTestId("message-queue-auto-sort-merge-enabled");
  const activate = (control: Locator) => (mobile ? control.tap() : control.click());
  const save = async () => {
    const response = waitForHttp(page, "PATCH", /\/api\/v1\/system\/message-queue\/settings$/);
    await activate(
      page.getByTestId("settings-floating-save").getByRole("button", { name: "Save changes" }),
    );
    const result = await response;
    expect(result.status()).toBe(200);
    return result.request().postDataJSON();
  };
  await expect(sort).not.toBeChecked();
  await expect(sort).toBeEnabled();
  if (mobile) {
    const target = page.getByTestId("message-queue-auto-sort-merge-touch-target");
    const box = await target.boundingBox();
    expect(Math.round(box!.width)).toBeGreaterThanOrEqual(44);
    expect(Math.round(box!.height)).toBeGreaterThanOrEqual(44);
  }
  await activate(sort);
  await activate(merge);
  await expect(sort).toBeDisabled();
  await expect(sort).not.toBeChecked();
  expect(await save()).toEqual({ auto_merge_enabled: false });
  expect(
    (await requestMessageQueueSettings(apiClient, "GET")).settings.auto_sort_merge_enabled,
  ).toBe(false);
  await activate(merge);
  await expect(sort).not.toBeChecked();
  expect(await save()).toEqual({ auto_merge_enabled: true });
  await activate(sort);
  expect(await save()).toEqual({ auto_sort_merge_enabled: true });
  await page.reload();
  await openTaskBehaviorRuntime(page);
  await expect(sort).toBeChecked();
  await activate(merge);
  await expect(sort).toBeDisabled();
  await expect(sort).not.toBeChecked();
  expect(await save()).toEqual({ auto_merge_enabled: false });
  expect(
    (await requestMessageQueueSettings(apiClient, "GET")).settings.auto_sort_merge_enabled,
  ).toBe(true);
  await page.reload();
  await openTaskBehaviorRuntime(page);
  await expect(sort).toBeDisabled();
  await activate(merge);
  await expect(sort).toBeChecked();
  await activate(page.getByTestId("settings-floating-save").getByRole("button", { name: "Reset" }));
  await expect(merge).not.toBeChecked();
  await expect(sort).toBeDisabled();
  await activate(merge);
  expect(await save()).toEqual({ auto_merge_enabled: true });
  await page.reload();
  await openTaskBehaviorRuntime(page);
  await expect(sort).toBeChecked();
  if (mobile) await assertNoDocumentHorizontalOverflow(page);
}
