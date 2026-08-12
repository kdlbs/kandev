import { test, expect } from "../../fixtures/test-base";

/**
 * Regression guard for a Card-wrapping bug: `BuiltinActionRow` used fixed
 * 420px/240px widths that, combined with the shared `Card`'s
 * `overflow-hidden`, pushed the model selector and edit button outside the
 * visible card on narrow viewports — clipped and unreachable rather than
 * stacked. See PR #1654 review discussion.
 */
test.describe("Mobile utility agents action rows", () => {
  test("repairs an unavailable action with the default by touch", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const agentId = "builtin-commit-message";
    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((agent) => agent.profiles ?? [])
      .find((candidate) => candidate.id === seedData.agentProfileId);
    if (!profile) throw new Error(`seed profile ${seedData.agentProfileId} was not found`);
    let savedBinding: Record<string, unknown> | undefined;
    const unavailable = {
      id: agentId,
      name: "commit-message",
      description: "Generate a commit message.",
      prompt: "Generate a commit message.",
      builtin: true,
      enabled: false,
      agent_id: "",
      model: "",
      agent_profile_id: "",
      profile_binding_state: "unconfigured",
    };

    await testPage.route("**/api/v1/user/settings", (route) => {
      if (route.request().method() !== "GET") return route.continue();
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: { default_utility_agent_profile_id: seedData.agentProfileId },
        }),
      });
    });
    await testPage.route(`**/api/v1/utility/agents/${agentId}`, async (route) => {
      if (route.request().method() !== "PATCH") return route.continue();
      savedBinding = route.request().postDataJSON() as Record<string, unknown>;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ...unavailable, ...savedBinding }),
      });
    });
    await testPage.route("**/api/v1/utility/agents", (route) => {
      if (route.request().method() !== "GET") return route.continue();
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ agents: [unavailable] }),
      });
    });

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    const card = testPage.getByTestId("utility-actions-card");
    const row = testPage.getByTestId(`utility-action-row-${agentId}`);
    const picker = row.getByTestId(`utility-profile-picker-action-${agentId}`);
    await picker.tap();
    const dropdown = testPage.getByTestId(`utility-profile-picker-action-${agentId}-dropdown`);
    await dropdown.locator('[data-value="__USE_DEFAULT__"]').tap();

    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).tap();
    await expect(floatingSave).not.toBeVisible({ timeout: 10_000 });
    expect(savedBinding).toMatchObject({
      agent_profile_id: "",
      profile_binding_state: "inherit",
      enabled: true,
    });
    expect(await card.evaluate((element) => element.scrollWidth > element.clientWidth + 1)).toBe(
      false,
    );
    await prCapture.screenshot("utility-action-inherits-default-mobile", {
      caption: "Default inheritance remains reachable on a narrow touch viewport.",
    });
  });

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
    await testPage.route("**/api/v1/user/settings", (route) => {
      if (route.request().method() !== "GET") return route.continue();
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: { default_utility_agent_profile_id: seedData.agentProfileId },
        }),
      });
    });
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
          agent_profile_id: "",
          profile_binding_state: "inherit",
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
              agent_profile_id: "",
              profile_binding_state: "inherit",
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
    const option = dropdown.locator(`[data-value="${profile.id}"]`);
    await expect(option).toHaveAttribute("role", "option");
    await expect(option).toContainText(profileLabel);
    await expect(option.getByTestId("utility-profile-agent-icon")).toBeVisible();
    await option.tap();
    await expect(select).toContainText(profileLabel);

    await row.getByRole("button").last().click();
    await expect(testPage.getByRole("dialog")).toBeVisible();
    await expect(testPage.getByText("Edit Utility Agent")).toBeVisible();
  });
});
