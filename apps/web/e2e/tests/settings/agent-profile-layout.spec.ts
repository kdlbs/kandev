import { test, expect } from "../../fixtures/test-base";

test.describe("Agent settings profile layout", () => {
  test("keeps profile creation in the agent card header", async ({ testPage, apiClient }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent || agent.profiles.length === 0) {
      throw new Error("The E2E fixture must provide a configured agent profile");
    }

    await testPage.goto("/settings/agents");

    const card = testPage.getByTestId(`agent-group-${agent.name}`);
    const header = card.getByTestId(`agent-card-header-${agent.name}`);
    const newProfile = header.getByTestId(`new-profile-${agent.name}`);
    await expect(newProfile).toBeVisible({ timeout: 15_000 });
    await expect(newProfile).toHaveAttribute(
      "href",
      `/settings/agents/${encodeURIComponent(agent.name)}?mode=create`,
    );

    // The action belongs to the card header, not the profile-list body.
    await expect(
      card.getByTestId(`agent-profiles-${agent.name}`).getByTestId(`new-profile-${agent.name}`),
    ).toHaveCount(0);
    await expect(card.getByText(/^\d+ profiles?$/)).toHaveCount(0);

    await newProfile.click();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/agents/${encodeURIComponent(agent.name)}\\?mode=create$`),
    );
  });

  test("orders installed-agent actions with refresh before agent creation", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/agents");

    const actions = testPage.getByTestId("installed-agents-actions");
    await expect(actions).toBeVisible({ timeout: 15_000 });
    await expect(actions.locator("[data-testid]")).toHaveCount(3);
    await expect(
      actions
        .locator("[data-testid]")
        .evaluateAll((elements) => elements.map((element) => element.getAttribute("data-testid"))),
    ).resolves.toEqual(["open-host-shell", "rescan-agents-button", "new-agent-button"]);
  });
});
