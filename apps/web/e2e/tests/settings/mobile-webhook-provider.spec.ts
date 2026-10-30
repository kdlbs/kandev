import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { expectTouchControl } from "../../helpers/control-sizing";
import type { NotificationProvider } from "../../../lib/types/http";

const PROVIDERS = "/api/v1/notification-providers";

test("creates, edits, and removes a webhook provider with touch-sized controls", async ({
  testPage,
  apiClient,
}) => {
  const name = `E2E mobile webhook ${Date.now()}`;
  const url = "https://hooks.example.test/kandev/mobile";
  const headers = "X-Tenant: mobile-edited";
  let providerId: string | undefined;

  try {
    await testPage.goto("/settings/preferences/notifications");

    const addProvider = testPage.getByRole("button", {
      name: "Add Webhook Provider",
      exact: true,
    });
    await expect(addProvider).toBeVisible();
    await expectTouchControl(addProvider, "Add webhook provider");
    await addProvider.tap();
    await testPage.getByPlaceholder("Provider name").fill(name);
    await testPage.getByPlaceholder("Webhook URL").fill(url);

    const createRequest = waitForHttp(testPage, "POST", new RegExp(`${PROVIDERS}$`));
    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true });
    await expectTouchControl(saveButton, "Save changes");
    await saveButton.tap();
    const createResponse = await createRequest;
    expect(createResponse.ok()).toBe(true);
    providerId = ((await createResponse.json()) as NotificationProvider).id;

    const providerName = testPage.locator("div.font-medium").filter({ hasText: name });
    const card = providerName.locator("xpath=ancestor::div[contains(@class,'rounded-lg')][1]");
    const edit = card.getByRole("button", { name: "Edit", exact: true });
    const remove = card.getByRole("button", { name: "Remove", exact: true });
    await expect(providerName).toBeVisible();
    await expectTouchControl(edit, "Edit webhook provider");
    await expectTouchControl(remove, "Remove webhook provider");
    await edit.tap();

    await testPage.getByPlaceholder("Headers, one per line (Name: value)").fill(headers);
    await testPage.getByRole("button", { name: "Done", exact: true }).tap();

    const updateRequest = waitForHttp(testPage, "PATCH", new RegExp(`${PROVIDERS}/${providerId}$`));
    await expectTouchControl(saveButton, "Save changes");
    await saveButton.tap();
    const updateResponse = await updateRequest;
    expect(updateResponse.ok()).toBe(true);

    const listed = await apiClient.rawRequest("GET", PROVIDERS);
    expect(listed.ok).toBe(true);
    const { providers } = (await listed.json()) as { providers: NotificationProvider[] };
    expect(providers.find((provider) => provider.id === providerId)?.config).toMatchObject({
      url,
      headers: { "X-Tenant": "mobile-edited" },
    });

    await remove.tap();
    const deleteRequest = waitForHttp(
      testPage,
      "DELETE",
      new RegExp(`${PROVIDERS}/${providerId}$`),
    );
    await expectTouchControl(saveButton, "Save changes");
    await saveButton.tap();
    const deleteResponse = await deleteRequest;
    expect(deleteResponse.ok()).toBe(true);
    providerId = undefined;
  } finally {
    if (providerId) {
      await apiClient.rawRequest("DELETE", `${PROVIDERS}/${providerId}`);
    }
  }
});
