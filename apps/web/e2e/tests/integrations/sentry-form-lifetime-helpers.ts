import { expect, type Locator, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForHttp } from "../../helpers/causal-waits";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

const INSTANCE_URL = "**/api/v1/sentry/instances**";
const INSTANCE_PATH = /\/api\/v1\/sentry\/instances(?:\/[^/]+)?$/;
const OLD = {
  name: "Submitted Sentry",
  url: "https://submitted.sentry.test",
  secret: "sntrys_synthetic_old",
};
const NEW = {
  name: "Newer Sentry draft",
  url: "https://newer.sentry.test",
  secret: "sntrys_synthetic_new",
};

type Options = { touch: boolean; kind: "add" | "edit" };

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1, AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.2
// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.3, AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.4
export async function exerciseSentryFormLifetime(
  page: Page,
  api: ApiClient,
  capture: PrAssetCapture,
  options: Options,
) {
  const workspaceName = `Sentry lifetime ${options.kind} ${crypto.randomUUID()}`;
  const workspace = await api.createWorkspace(workspaceName);
  const admitted = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const payloads: Array<{ method: string; url: string; body: Record<string, string> }> = [];
  let pendingRoute: Promise<void> | undefined;
  const method = options.kind === "add" ? "POST" : "PUT";
  const press = async (control: Locator) => (options.touch ? control.tap() : control.click());
  const intercept = async (route: Route) => {
    if (!["POST", "PUT"].includes(route.request().method())) return route.continue();
    payloads.push({
      method: route.request().method(),
      url: route.request().url(),
      body: route.request().postDataJSON(),
    });
    if (payloads.length !== 1) return route.continue();
    pendingRoute = (async () => {
      try {
        const response = await route.fetch();
        expect(response.ok()).toBe(true);
        admitted.resolve();
        await release.promise;
        await route.fulfill({ response });
      } catch (error) {
        admitted.reject(error);
        throw error;
      }
    })();
    return pendingRoute;
  };
  try {
    const original = await api.createSentryInstance({
      workspaceId: workspace.id,
      name: "Original Sentry",
      secret: "sntrys_synthetic_fixture",
    });
    const settings = new SentrySettingsPage(page);
    await settings.goto(workspace.id);
    if (options.kind === "add") await press(settings.addInstanceButton);
    else await press(settings.cardByName(original.name).getByTestId("sentry-instance-edit-button"));
    for (const [field, value] of Object.entries(OLD))
      await page.getByTestId(`sentry-${options.kind}-${field}-input`).fill(value);
    await page.route(INSTANCE_URL, intercept);
    const floating = page.getByTestId("settings-floating-save");
    const save = floating.getByRole("button", { name: "Save changes", exact: true });
    await press(save);
    await admitted.promise;
    expect(payloads).toHaveLength(1);
    expect(payloads[0].method).toBe(method);
    expect(new URL(payloads[0].url).searchParams.get("workspace_id")).toBe(workspace.id);
    expect(new URL(payloads[0].url).pathname).toBe(
      `/api/v1/sentry/instances${options.kind === "edit" ? `/${original.id}` : ""}`,
    );
    expect(payloads[0].body).toEqual(expect.objectContaining(OLD));
    expect(
      (await api.listSentryInstances(workspace.id)).some(
        (row) => row.name === OLD.name && row.url === OLD.url,
      ),
    ).toBe(true);
    await press(page.getByTestId(`sentry-${options.kind}-cancel-button`));
    if (options.kind === "add") await press(settings.addInstanceButton);
    else await press(settings.cardByName(original.name).getByTestId("sentry-instance-edit-button"));
    for (const [field, value] of Object.entries(NEW))
      await page.getByTestId(`sentry-${options.kind}-${field}-input`).fill(value);
    const name = page.getByTestId(`sentry-${options.kind}-name-input`);
    await press(name);
    await page.keyboard.press("Home");
    await page.keyboard.press("ArrowRight");
    const selection = await name.evaluate((el) => [
      (el as HTMLInputElement).selectionStart,
      (el as HTMLInputElement).selectionEnd,
    ]);
    const ack = waitForHttp(page, method, INSTANCE_PATH);
    release.resolve();
    await ack;
    await pendingRoute;
    for (const [field, value] of Object.entries(NEW))
      await expect(page.getByTestId(`sentry-${options.kind}-${field}-input`)).toHaveValue(value);
    await expect(name).toBeFocused();
    expect(
      await name.evaluate((el) => [
        (el as HTMLInputElement).selectionStart,
        (el as HTMLInputElement).selectionEnd,
      ]),
    ).toEqual(selection);
    await expect(page.getByTestId(`sentry-${options.kind}-form`)).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    await expect(save).toBeEnabled();
    await expect(page.getByTestId(`sentry-${options.kind}-cancel-button`)).toBeEnabled();
    expect(payloads).toHaveLength(1);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    await capture.screenshot(`sentry-${options.kind}-newer-draft`, {
      caption: "Newer Sentry fields remain dirty after an earlier accepted write completes.",
    });
    if (options.kind === "add") {
      const nextAck = waitForHttp(page, "POST", INSTANCE_PATH);
      await press(save);
      await nextAck;
      await expect(page.getByTestId("sentry-add-form")).toHaveCount(0);
      expect(payloads).toHaveLength(2);
      expect(payloads[1].body).toEqual(expect.objectContaining(NEW));
      await page.reload();
      await expect(settings.cardByName(NEW.name)).toBeVisible();
    } else {
      await press(page.getByTestId("sentry-edit-cancel-button"));
      await expect(floating).toHaveCount(0);
      expect(payloads).toHaveLength(1);
      await page.reload();
      await expect(settings.cardByName(OLD.name)).toBeVisible();
      await expect(settings.cardByName(NEW.name)).toHaveCount(0);
    }
  } finally {
    release.resolve();
    try {
      await pendingRoute;
    } finally {
      await page.unroute(INSTANCE_URL, intercept);
      await api.deleteWorkspace(workspace.id, workspaceName);
    }
  }
}
