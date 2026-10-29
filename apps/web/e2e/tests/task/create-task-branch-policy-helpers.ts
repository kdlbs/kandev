import { expect, type Locator } from "@playwright/test";

export async function expectPolicyOptionUsesOneLine(option: Locator, policyName: string) {
  const marker = option.getByText("Policy", { exact: true });
  const name = option.getByText(policyName, { exact: true });
  await expect
    .poll(
      async () => {
        const [markerBox, nameBox] = await Promise.all([marker.boundingBox(), name.boundingBox()]);
        if (!markerBox || !nameBox) return Number.POSITIVE_INFINITY;
        return Math.abs(markerBox.y + markerBox.height / 2 - (nameBox.y + nameBox.height / 2));
      },
      { message: "Policy marker and name must be visible on one line" },
    )
    .toBeLessThan(3);
  await expect(option.getByText(/Base:/)).toHaveCount(0);
}
