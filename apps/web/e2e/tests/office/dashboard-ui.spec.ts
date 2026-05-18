import { test, expect } from "../../fixtures/office-fixture";

test.describe("Dashboard UI", () => {
  test("dashboard renders chart sections", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Run Activity")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Success Rate")).toBeVisible();
  });

  test("dashboard renders recent tasks section", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByRole("heading", { name: "Recent Tasks" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("dashboard renders recent activity section", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByRole("heading", { name: "Recent Activity" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("dashboard shows agent count metric", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
  });
});
