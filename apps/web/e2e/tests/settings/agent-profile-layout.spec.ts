import { test, expect } from "../../fixtures/test-base";
import { seededAgentProfile } from "../../helpers/seeded-agent-profile";

test.describe("Agent settings profile layout", () => {
  test("keeps profile creation in the agent card header", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Select the seeded profile's owner by identity: list order changes with
    // discovery, and a virtual family such as Dynamic can sort first with no
    // profiles of its own.
    const { agent } = await seededAgentProfile(apiClient, seedData.agentProfileId);

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
    await expect(
      actions
        .locator("[data-testid]")
        .evaluateAll((elements) => elements.map((element) => element.getAttribute("data-testid"))),
    ).resolves.toEqual([
      "agent-options-trigger",
      "open-host-shell",
      "rescan-agents-button",
      "new-agent-button",
    ]);
  });
});
