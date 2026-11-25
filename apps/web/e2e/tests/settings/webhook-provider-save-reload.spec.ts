import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import type { NotificationProvider } from "../../../lib/types/http";

const PROVIDERS = "/api/v1/notification-providers";

test("saves a webhook provider and restores it after reload", async ({ testPage, apiClient }) => {
  const name = `E2E webhook ${Date.now()}`;
  const url = "https://hooks.example.test/kandev/e2e";
  const headers = "X-Tenant: e2e\nX-Trace: abc123";
  const secret = "e2e-secret";
  let id: string | undefined;

  try {
    await testPage.goto("/settings/preferences/notifications");

    await testPage.getByRole("button", { name: "Add Webhook Provider", exact: true }).click();
    await testPage.getByPlaceholder("Provider name").fill(name);
    await testPage.getByPlaceholder("Webhook URL").fill(url);
    await testPage.getByPlaceholder("Headers, one per line (Name: value)").fill(headers);
    await testPage.getByPlaceholder("Secret (optional)").fill(secret);

    const created = waitForHttp(testPage, "POST", new RegExp(`${PROVIDERS}$`));
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    const response = await created;
    expect(response.ok()).toBe(true);
    id = ((await response.json()) as NotificationProvider).id;

    const cardName = testPage.locator("div.font-medium").filter({ hasText: name });
    await expect(cardName).toBeVisible();
    await expect(
      testPage.getByTestId("settings-floating-save").getByRole("button", { name: "Save changes" }),
    ).toHaveCount(0);

    await testPage.reload();

    await expect(cardName).toBeVisible();
    await cardName
      .locator("xpath=ancestor::div[contains(@class,'justify-between')][1]")
      .getByRole("button", { name: "Edit", exact: true })
      .click();
    await expect(testPage.getByPlaceholder("Provider name")).toHaveValue(name);
    await expect(testPage.getByPlaceholder("Webhook URL")).toHaveValue(url);
    await expect(testPage.getByPlaceholder("Headers, one per line (Name: value)")).toHaveValue(
      headers,
    );
    await expect(testPage.getByPlaceholder("Secret (optional)")).toHaveValue(secret);
    await testPage.getByRole("button", { name: "Cancel", exact: true }).click();

    const listed = await apiClient.rawRequest("GET", PROVIDERS);
    expect(listed.ok).toBe(true);
    const { providers } = (await listed.json()) as { providers: NotificationProvider[] };
    const persisted = providers.find((provider) => provider.id === id);
    expect(persisted?.type).toBe("webhook");
    expect(persisted?.config).toMatchObject({
      url,
      headers: { "X-Tenant": "e2e", "X-Trace": "abc123" },
      secret,
    });
    expect(Array.isArray(persisted?.events)).toBe(true);
  } finally {
    if (id) expect((await apiClient.rawRequest("DELETE", `${PROVIDERS}/${id}`)).ok).toBe(true);
  }
});
