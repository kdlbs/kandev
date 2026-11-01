import { expect, type Locator } from "@playwright/test";

export async function expectMCPSectionHierarchy(explorer: Locator, inheritedName: string) {
  const inherited = explorer.getByRole("heading", { name: "Selected for this task", exact: true });
  const additional = explorer.getByRole("heading", { name: "Additional servers", exact: true });
  const server = explorer.getByTestId("mcp-inherited-group").getByText(inheritedName, {
    exact: true,
  });
  await expect(inherited).toHaveCSS("text-transform", "uppercase");
  await expect(additional).toHaveCSS("text-transform", "uppercase");
  const [headerBox, serverBox, additionalBox] = await Promise.all([
    inherited.boundingBox(),
    server.boundingBox(),
    additional.boundingBox(),
  ]);
  expect(headerBox).not.toBeNull();
  expect(serverBox).not.toBeNull();
  expect(additionalBox).not.toBeNull();
  expect(serverBox!.x - headerBox!.x).toBeGreaterThanOrEqual(12);
  expect(Math.abs(additionalBox!.x - headerBox!.x)).toBeLessThanOrEqual(1);
  expect(serverBox!.y).toBeGreaterThan(headerBox!.y + headerBox!.height);
  await expect(explorer.getByTestId("mcp-inherited-group")).toHaveCSS(
    "background-color",
    "rgba(0, 0, 0, 0)",
  );
}
