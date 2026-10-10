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
    expect((await dialog.boundingBox())!.height).toBeLessThanOrEqual(720);
    await expect(testPage.getByRole("heading", { name: "AI Agents" })).toBeVisible();
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();

    const agentTrigger = testPage.getByRole("button", { name: /^Mock / });
    await expect(agentTrigger).toBeVisible();
    await agentTrigger.tap();
    const agentRow = testPage.getByTestId("onboarding-agent-setup-fields");
    await expect(agentRow).toBeVisible();
    const passthroughTarget = agentRow.getByTestId("onboarding-agent-passthrough-field");
    const targetBounds = await passthroughTarget.boundingBox();
    expect(targetBounds).not.toBeNull();
    expect(targetBounds!.height).toBeGreaterThanOrEqual(44);
    const passthrough = agentRow.getByRole("switch", { name: "TUI Passthrough" });
    await expect(passthrough).toBeVisible();
    await passthroughTarget.tap({ position: { x: 20, y: targetBounds!.height / 2 } });
    await expect(passthrough).toBeChecked();
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
    await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
    if ((await selector.getAttribute("aria-expanded")) !== "true") await selector.tap();
    await testPage.getByTestId("config-option-trigger-effort").tap();
    await testPage.getByRole("button", { name: "Max", exact: true }).tap();
    await expect(selector).toContainText("Max");
    await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
    await selector.tap();

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
    await expect(selector).toContainText("Max");
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();
  });
});
