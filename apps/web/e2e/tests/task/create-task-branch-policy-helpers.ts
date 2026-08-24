import { expect, type Locator } from "@playwright/test";

export async function expectPolicyOptionContentNotToOverlap(option: Locator, policyName: string) {
  const marker = option.getByText("Policy", { exact: true });
  const name = option.getByText(policyName, { exact: true });
  const summary = option.getByText(/Base:/);
  const [markerBox, nameBox, summaryBox] = await Promise.all([
    marker.boundingBox(),
    name.boundingBox(),
    summary.boundingBox(),
  ]);

  expect(markerBox).not.toBeNull();
  expect(nameBox).not.toBeNull();
  expect(summaryBox).not.toBeNull();

  for (const identityBox of [markerBox!, nameBox!]) {
    const overlaps =
      identityBox.x < summaryBox!.x + summaryBox!.width &&
      identityBox.x + identityBox.width > summaryBox!.x &&
      identityBox.y < summaryBox!.y + summaryBox!.height &&
      identityBox.y + identityBox.height > summaryBox!.y;
    expect(overlaps).toBe(false);
  }
}
