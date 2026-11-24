import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import {
  IDENTITY_LINK_CODE,
  mockHumanResponsesPlugin,
  mockIdentityLink,
} from "../../helpers/plugin-identity-links";

test.describe("plugin linked chat accounts", () => {
  test("generates a link code, lists accounts, revokes one, and refreshes on focus", async ({
    testPage: page,
  }) => {
    const state = await mockHumanResponsesPlugin(page, [mockIdentityLink("link-1", "alice#0001")]);
    await installFixturePlugin(page);
    await page.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();

    const card = page.getByTestId("plugin-identity-links");
    await expect(card).toBeVisible();
    await expect(card.getByTestId("plugin-identity-link-link-1")).toContainText("alice#0001");

    await card.getByTestId("plugin-identity-link-code-create").click();
    await expect(card.getByTestId("plugin-identity-link-code")).toContainText(IDENTITY_LINK_CODE);

    // A code is redeemed in the chat app, so the list refreshes on focus.
    state.links = [...state.links, mockIdentityLink("link-2", "bob#0002")];
    await page.evaluate(() => window.dispatchEvent(new Event("focus")));
    await expect(card.getByTestId("plugin-identity-link-link-2")).toContainText("bob#0002");

    await card.getByTestId("plugin-identity-link-revoke-link-1").click();
    await expect(card.getByTestId("plugin-identity-link-link-1")).toHaveCount(0);
    await expect(card.getByTestId("plugin-identity-link-link-2")).toBeVisible();
  });
});
