import { expect, type Locator } from "@playwright/test";

export async function expectMCPInheritanceBadges(
  explorer: Locator,
  definitionId: string,
  _taskTitle: string,
  ownerIds: string[],
  mobile: boolean,
) {
  const row = explorer.getByTestId(`mcp-inherited-server-${definitionId}`);
  await expect(row).toContainText("Shared guides for the Atlas infrastructure team.");
  const details = row.getByTestId("mcp-definition-badges");
  await expect(details).toContainText("Remote endpoint");
  await expect(details).toContainText("Server-sent events");
  await expect(details).toContainText("Custom");
  await expect(
    explorer
      .getByTestId("mcp-origin-section-task")
      .getByTestId(`mcp-inherited-server-${definitionId}`),
  ).toHaveCount(1);
  await expect(row).toContainText("Also selected by");
  await expect(row.getByTestId("mcp-origin-badge")).toHaveCount(0);
  await expect(row).toContainText(/Profile: .+/);
  await expect(row).toContainText(/Repository: .+/);
  for (const id of ownerIds) await expect(row).not.toContainText(id);
  const bounds = (await row.boundingBox())!;
  for (const badge of await row.locator('[data-slot="badge"]').all()) {
    const box = (await badge.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(bounds.x);
    expect(box.x + box.width).toBeLessThanOrEqual(bounds.x + bounds.width + 1);
  }
  if (mobile) {
    // Stress a narrower content column while retaining the full source name.
    await row.evaluate((element) => {
      element.style.maxWidth = "260px";
    });
    try {
      expect(
        (await row.locator(":scope > div > span").first().boundingBox())!.height,
      ).toBeGreaterThanOrEqual(32);
    } finally {
      await row.evaluate((element) => element.style.removeProperty("max-width"));
    }
  }
}
