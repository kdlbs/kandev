import { expect, type Locator } from "@playwright/test";

export async function expectPolicyOptionUsesOneLine(option: Locator, policyName: string) {
  const marker = option.getByText("Policy", { exact: true });
  const name = option.getByText(policyName, { exact: true });
  const [markerBox, nameBox] = await Promise.all([marker.boundingBox(), name.boundingBox()]);

  expect(markerBox).not.toBeNull();
  expect(nameBox).not.toBeNull();
  expect(
    Math.abs(markerBox!.y + markerBox!.height / 2 - (nameBox!.y + nameBox!.height / 2)),
  ).toBeLessThan(3);
  await expect(option.getByText(/Base:/)).toHaveCount(0);
}
