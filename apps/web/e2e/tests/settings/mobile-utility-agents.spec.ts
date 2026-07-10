import { test, expect } from "../../fixtures/test-base";

/**
 * Regression guard for a Card-wrapping bug: `BuiltinActionRow` used fixed
 * 420px/240px widths that, combined with the shared `Card`'s
 * `overflow-hidden`, pushed the model selector and edit button outside the
 * visible card on narrow viewports — clipped and unreachable rather than
 * stacked. See PR #1654 review discussion.
 */
test.describe("Mobile utility agents action rows", () => {
  test("model select and edit button stay reachable on a narrow viewport", async ({ testPage }) => {
    await testPage.route("**/api/v1/utility/inference-agents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "claude",
              name: "claude",
              display_name: "Claude",
              status: "ok",
              models: [
                { id: "claude-fast", name: "Claude Fast", description: "", is_default: true },
              ],
            },
          ],
        }),
      }),
    );
    await testPage.route("**/api/v1/utility/agents/builtin-commit-message", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "builtin-commit-message",
          name: "commit-message",
          description: "Generate a commit message.",
          builtin: true,
          enabled: true,
          agent_id: "claude",
          model: "claude-fast",
        }),
      }),
    );
    await testPage.route("**/api/v1/utility/agents", (route) => {
      if (route.request().method() !== "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ success: true }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "builtin-commit-message",
              name: "commit-message",
              description: "Generate a commit message.",
              builtin: true,
              enabled: true,
              agent_id: "",
              model: "",
            },
          ],
        }),
      });
    });

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    const card = testPage.getByTestId("utility-actions-card");
    await expect(card).toBeVisible();

    // The card must not have to clip its own content to stay within bounds
    // — regression guard for the fixed 420px/240px row colliding with the
    // shared Card's `overflow-hidden`.
    const isOverflowing = await card.evaluate((el) => el.scrollWidth > el.clientWidth + 1);
    expect(isOverflowing).toBe(false);

    // The model selector and edit button must stay clickable, not clipped
    // outside the card.
    const row = testPage.getByTestId("utility-action-row-builtin-commit-message");
    const select = row.getByRole("combobox");
    await select.click();
    await testPage.getByRole("option", { name: "Claude Fast", exact: true }).click();
    await expect(select).toContainText("Claude Fast");

    await row.getByRole("button").click();
    await expect(testPage.getByRole("dialog")).toBeVisible();
    await expect(testPage.getByText("Edit Utility Agent")).toBeVisible();
  });
});
