import { expect, test } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Agent recovery control on mobile", () => {
  test("offers a touch-sized control that returns a paused agent to idle", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.updateAgentStatus(officeSeed.agentId, "paused", "Manually paused for testing");

    await testPage.goto(`/office/agents/${officeSeed.agentId}/dashboard`);
    const recoveryControl = testPage.getByTestId("agent-recovery-control");
    await expect(recoveryControl).toBeVisible({ timeout: 10_000 });

    const documentWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    expect(documentWidth).toBeLessThanOrEqual(viewportWidth);

    const box = await recoveryControl.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);

    const recovered = waitForHttp(
      testPage,
      "PATCH",
      new RegExp(`/agents/${officeSeed.agentId}/status$`),
    );
    await recoveryControl.tap();
    await recovered;

    await expect(recoveryControl).toBeHidden();

    const agent = await officeApi.getAgent(officeSeed.agentId);
    expect((agent as Record<string, unknown>).status).toBe("idle");
  });
});
