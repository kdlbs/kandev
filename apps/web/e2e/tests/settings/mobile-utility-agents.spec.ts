import { test, expect } from "../../fixtures/test-base";

/**
 * Regression guard for a Card-wrapping bug: `BuiltinActionRow` used fixed
 * 420px/240px widths that, combined with the shared `Card`'s
 * `overflow-hidden`, pushed the model selector and edit button outside the
 * visible card on narrow viewports — clipped and unreachable rather than
 * stacked. See PR #1654 review discussion.
 */
test.describe("Mobile utility agents action rows", () => {
  test("profile select and edit button stay reachable on a narrow viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((agent) => agent.profiles ?? [])
      .find((candidate) => candidate.id === seedData.agentProfileId);
    if (!profile) throw new Error(`seed profile ${seedData.agentProfileId} was not found`);
    const profileLabel = `${profile.agentDisplayName} • ${profile.name}`;
    await testPage.route("**/api/v1/utility/agents/builtin-commit-message", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "builtin-commit-message",
          name: "commit-message",
          description: "Generate a commit message.",
          prompt: "Generate a commit message.",
          builtin: true,
          enabled: true,
          agent_id: "",
          model: "",
          agent_profile_id: seedData.agentProfileId,
          profile_binding_state: "explicit",
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
              prompt: "Generate a commit message.",
              builtin: true,
              enabled: true,
              agent_id: "",
              model: "",
              agent_profile_id: seedData.agentProfileId,
              profile_binding_state: "explicit",
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

    // The profile selector must also stay a comfortably usable width — not
    // just "not clipped" but not squeezed illegibly thin either. The row
    // stacks below `md`, so the select gets the card's full content width
    // to itself (minus the edit button), not a slice shared with the name
    // column.
    const row = testPage.getByTestId("utility-action-row-builtin-commit-message");
    const select = row.getByTestId("utility-profile-picker-action-builtin-commit-message");
    const selectBox = await select.boundingBox();
    expect(selectBox).not.toBeNull();
    expect(selectBox!.width).toBeGreaterThanOrEqual(150);

    // The profile selector and edit button must stay clickable, not clipped
    // outside the card.
    await expect(select).toContainText(profileLabel);
    await select.tap();
    const dropdown = testPage.getByTestId(
      "utility-profile-picker-action-builtin-commit-message-dropdown",
    );
    await dropdown.getByPlaceholder("Search agent profiles...").fill(profile.agentDisplayName);
    const option = dropdown.getByRole("option", { name: profileLabel, exact: true });
    await expect(option.getByTestId("utility-profile-agent-icon")).toBeVisible();
    await option.tap();
    await expect(select).toContainText(profileLabel);

    await row.getByRole("button").last().click();
    await expect(testPage.getByRole("dialog")).toBeVisible();
    await expect(testPage.getByText("Edit Utility Agent")).toBeVisible();
  });
});
