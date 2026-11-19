import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { expectTouchControl } from "../../helpers/control-sizing";

test("keeps a long disable conflict dialog inside a short phone viewport", async ({
  testPage,
  apiClient,
}) => {
  await testPage.setViewportSize({ width: 320, height: 480 });
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
  const profile = await apiClient.createAgentProfile(agent.id, "Mobile disable conflict", {
    model: agent.profiles[0].model,
  });
  const profileRoute = new RegExp(`/api/v1/agent-profiles/${profile.id}(?:\\?.*)?$`);

  try {
    await testPage.route(profileRoute, async (route) => {
      if (route.request().method() !== "PATCH") {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({
          error: "agent profile is in use",
          utility_agents: Array.from({ length: 8 }, (_, index) => ({
            id: `utility-${index}`,
            name: `Utility binding with a long name ${index}`,
          })),
          dynamic_profiles: Array.from({ length: 8 }, (_, index) => ({
            id: `dynamic-${index}`,
            name: `Dynamic profile with a long name ${index}`,
          })),
        }),
      });
    });

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    await testPage.getByTestId("profile-enabled-toggle").tap();

    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true });
    await expect(saveButton).toBeEnabled();
    const update = waitForHttp(
      testPage,
      "PATCH",
      new RegExp(`/api/v1/agent-profiles/${profile.id}$`),
    );
    await saveButton.tap();
    expect((await update).status()).toBe(409);

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Utility binding with a long name 0");
    await expect(dialog).toContainText("Dynamic profile with a long name 0");
    const viewport = await testPage.evaluate(() => ({
      width: window.innerWidth,
      height: window.innerHeight,
    }));
    const body = dialog.getByTestId("disable-profile-conflict-body");
    const bodyMetrics = await body.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
    }));
    expect(bodyMetrics.scrollHeight).toBeGreaterThan(bodyMetrics.clientHeight);
    await body.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });

    const footer = dialog.getByTestId("disable-profile-conflict-footer");
    await expect(footer).toBeVisible();
    const footerBounds = await footer.boundingBox();
    expect(footerBounds).not.toBeNull();
    expect(footerBounds!.y + footerBounds!.height).toBeLessThanOrEqual(viewport.height);
    for (const name of ["Cancel", "Disable profile"]) {
      const action = dialog.getByRole("button", { name, exact: true });
      await expectTouchControl(action, name);
      const receivesTap = await action.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        const hit = document.elementFromPoint(
          bounds.left + bounds.width / 2,
          bounds.top + bounds.height / 2,
        );
        return hit === element || element.contains(hit);
      });
      expect(receivesTap).toBe(true);
    }
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
    const bounds = await dialog.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.y).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height);
  } finally {
    await testPage.unroute(profileRoute);
    await apiClient.deleteAgentProfile(profile.id, true);
  }
});
