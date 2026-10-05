import { test, expect } from "../../fixtures/test-base";
// Runs under the mobile-chrome project (Pixel 5, viewport 393x851).
//
// Covers the first-run dialog's phone suppression and later larger-viewport
// availability. It is distinct from the office /office/setup wizard.

test.describe("First-run onboarding availability — mobile", () => {
  test("keeps the tour off phones without consuming it, including resize transitions", async ({
    testPage,
  }) => {
    // Undo the test-base init script that pre-marks onboarding completed —
    // otherwise the dialog never opens on `/`.
    await testPage.addInitScript(() => {
      localStorage.removeItem("kandev.onboarding.completed");
    });
    await testPage.goto("/");

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-kanban-layout")).toBeVisible();
    await expect(testPage.getByTestId("mobile-fab")).toBeEnabled();
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();

    await testPage.setViewportSize({ width: 768, height: 851 });
    await expect(dialog).toBeVisible();
    await expect(testPage.getByRole("heading", { name: "AI Agents" })).toBeVisible();
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();

    const agentTrigger = testPage.getByRole("button", { name: /^Mock / });
    await expect(agentTrigger).toBeVisible();
    await agentTrigger.tap();
    const agentRow = testPage.getByTestId("onboarding-agent-setup-fields");
    await expect(agentRow).toBeVisible();
    const refreshBtn = testPage.getByTestId("onboarding-agent-refresh-models");
    const box = await refreshBtn.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
    const selector = agentRow.getByRole("button", { name: "Profile start model settings" });
    await expect(selector).toBeEnabled({ timeout: 20_000 });
    await selector.tap();
    await testPage.getByRole("option", { name: "Mock Smart", exact: true }).tap();
    await expect(selector).toContainText("Mock Smart");

    await testPage.setViewportSize({ width: 393, height: 851 });
    await expect(dialog).toHaveCount(0);
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();

    await testPage.setViewportSize({ width: 768, height: 851 });
    await expect(dialog).toBeVisible();
    await expect(agentTrigger).toContainText("Mock Smart");
    await agentTrigger.tap();
    await expect(selector).toContainText("Mock Smart");
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();
  });
});
