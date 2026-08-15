import { test, expect } from "../../fixtures/test-base";

test.describe("Agent settings profile layout on mobile", () => {
  test("keeps creation reachable without horizontal overflow", async ({ testPage, apiClient }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent || agent.profiles.length === 0) {
      throw new Error("The E2E fixture must provide a configured agent profile");
    }

    await testPage.goto("/settings/agents");

    await expect
      .poll(
        async () =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        { timeout: 15_000 },
      )
      .toBe(true);

    const card = testPage.getByTestId(`agent-group-${agent.name}`);
    const newProfile = card
      .getByTestId(`agent-card-header-${agent.name}`)
      .getByTestId(`new-profile-${agent.name}`);
    await expect(newProfile).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(
        async () => {
          const box = await newProfile.boundingBox();
          return box ? Math.min(box.width, box.height) : null;
        },
        { timeout: 10_000 },
      )
      .toBeGreaterThanOrEqual(44);

    const actions = testPage.getByTestId("installed-agents-actions");
    for (const testId of ["open-host-shell", "rescan-agents-button", "new-agent-button"]) {
      const control = actions.getByTestId(testId);
      await expect(control).toBeVisible();
      await expect
        .poll(
          async () => {
            const box = await control.boundingBox();
            return box ? Math.min(box.width, box.height) : null;
          },
          { timeout: 10_000 },
        )
        .toBeGreaterThanOrEqual(44);
    }

    await newProfile.tap();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/agents/${encodeURIComponent(agent.name)}\\?mode=create$`),
    );
  });
});
